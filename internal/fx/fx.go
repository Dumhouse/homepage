// Package fx converts money between currencies for totals. Rates are the
// European Central Bank's daily reference rates (EUR base, ~30 currencies),
// cached in settings, plus manual rates for anything the ECB doesn't publish.
package fx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	KeyDefault = "currency"  // default currency, e.g. EUR
	KeyRates   = "fx_rates"  // cached ECB rates (JSON)
	KeyManual  = "fx_manual" // "UAH=0.0225\nXYZ=1.5" — value of 1 unit in EUR
	KeyAuto    = "fx_auto"   // "off" disables fetching
	ecbURL     = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"
)

// Settings is the subset of the settings store fx needs.
type Settings interface {
	Get(ctx context.Context, key, def string) (string, error)
	Set(ctx context.Context, actor, key, value string) error
}

// Cached is the stored ECB snapshot: units of each currency per 1 EUR.
type Cached struct {
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

// Table is a ready-to-use set of rates: units per 1 EUR.
type Table struct {
	Date   string
	Rates  map[string]float64
	Manual map[string]bool
}

// Endpoint can be pointed at a fake server in tests.
var Endpoint = ecbURL

var client = &http.Client{Timeout: 15 * time.Second}

// Fetch downloads today's ECB reference rates.
func Fetch(ctx context.Context) (*Cached, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange rates: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange rates: HTTP %d", res.StatusCode)
	}
	var doc struct {
		Cube struct {
			Cube struct {
				Time  string `xml:"time,attr"`
				Rates []struct {
					Currency string `xml:"currency,attr"`
					Rate     string `xml:"rate,attr"`
				} `xml:"Cube"`
			} `xml:"Cube"`
		} `xml:"Cube"`
	}
	if err := xml.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("exchange rates unreadable: %w", err)
	}
	out := &Cached{Date: doc.Cube.Cube.Time, Rates: map[string]float64{"EUR": 1}}
	for _, r := range doc.Cube.Cube.Rates {
		if v, err := strconv.ParseFloat(r.Rate, 64); err == nil && v > 0 {
			out.Rates[strings.ToUpper(r.Currency)] = v
		}
	}
	if len(out.Rates) < 2 {
		return nil, errors.New("exchange rates: empty answer")
	}
	return out, nil
}

// Refresh fetches and stores the ECB rates.
func Refresh(ctx context.Context, set Settings, actor string) (*Cached, error) {
	c, err := Fetch(ctx)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(c)
	return c, set.Set(ctx, actor, KeyRates, string(b))
}

// ParseManual reads "UAH=0.0225" lines: the value of one unit in EUR.
func ParseManual(s string) (map[string]float64, error) {
	out := map[string]float64{}
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' || r == ';' }) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		code := strings.ToUpper(strings.TrimSpace(k))
		f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(v), ",", "."), 64)
		if !ok || len(code) != 3 || err != nil || f <= 0 {
			return nil, fmt.Errorf("%q: write it as CODE=value of 1 unit in EUR, e.g. UAH=0.0225", line)
		}
		out[code] = f
	}
	return out, nil
}

// Load builds the rate table from the cached ECB rates and manual rates.
func Load(ctx context.Context, set Settings) Table {
	t := Table{Rates: map[string]float64{"EUR": 1}, Manual: map[string]bool{}}
	raw, _ := set.Get(ctx, KeyRates, "")
	var c Cached
	if json.Unmarshal([]byte(raw), &c) == nil {
		t.Date = c.Date
		for k, v := range c.Rates {
			t.Rates[k] = v
		}
	}
	m, _ := set.Get(ctx, KeyManual, "")
	if manual, err := ParseManual(m); err == nil {
		for k, eurPerUnit := range manual {
			t.Rates[k] = 1 / eurPerUnit
			t.Manual[k] = true
		}
	}
	return t
}

// Convert turns cents in `from` into cents in `to`. ok is false when a rate
// is missing.
func (t Table) Convert(cents int64, from, to string) (int64, bool) {
	from, to = strings.ToUpper(from), strings.ToUpper(to)
	if from == to || from == "" {
		return cents, true
	}
	rf, ok1 := t.Rates[from]
	rt, ok2 := t.Rates[to]
	if !ok1 || !ok2 || rf == 0 {
		return 0, false
	}
	return int64(math.Round(float64(cents) / rf * rt)), true
}

// Codes lists every currency with a rate, sorted, EUR first.
func (t Table) Codes() []string {
	out := []string{}
	for k := range t.Rates {
		if k != "EUR" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return append([]string{"EUR"}, out...)
}

// Loop refreshes the ECB rates once a day (and at start when they are
// missing or old) unless fx_auto is "off".
func Loop(ctx context.Context, set Settings) func() {
	ctx, cancel := context.WithCancel(ctx)
	var once sync.Once
	go func() {
		check := func() {
			if v, _ := set.Get(ctx, KeyAuto, "on"); v == "off" {
				return
			}
			if t := Load(ctx, set); t.Date != "" {
				if d, err := time.Parse("2006-01-02", t.Date); err == nil && time.Since(d) < 30*time.Hour {
					return
				}
			}
			_, _ = Refresh(ctx, set, "system:fx")
		}
		once.Do(check)
		tk := time.NewTicker(6 * time.Hour)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				check()
			}
		}
	}()
	return cancel
}

// HistEndpoint serves the ECB's last 90 days of reference rates; older dates
// use the full history (FullEndpoint, a zipped CSV fetched only when needed).
var (
	HistEndpoint = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist-90d.xml"
	FullEndpoint = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist.zip"
	histMu       sync.Mutex
	histCache    = map[string]map[string]float64{} // date -> currency -> units per EUR
	histFetched  = map[string]time.Time{}          // endpoint -> when
)

// TableOn returns the ECB rates of the last business day on or before date
// (weekends and holidays have no fixing), plus manual rates.
func TableOn(ctx context.Context, set Settings, date string) (Table, string, error) {
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return Table{}, "", fmt.Errorf("bad date %q", date)
	}
	endpoint := HistEndpoint
	if time.Since(day) > 85*24*time.Hour {
		endpoint = FullEndpoint
	}
	if err := loadHist(ctx, endpoint); err != nil {
		return Table{}, "", err
	}
	histMu.Lock()
	defer histMu.Unlock()
	for i := 0; i < 10; i++ {
		d := day.AddDate(0, 0, -i).Format("2006-01-02")
		if rates, ok := histCache[d]; ok {
			t := Table{Date: d, Rates: map[string]float64{"EUR": 1}, Manual: map[string]bool{}}
			for k, v := range rates {
				t.Rates[k] = v
			}
			m, _ := set.Get(ctx, KeyManual, "")
			if manual, err := ParseManual(m); err == nil {
				// Manual rates deliberately win, both here and in Load: the user
				// knows their bank's rate better than the ECB fix for a currency.
				for k, eurPerUnit := range manual {
					t.Rates[k] = 1 / eurPerUnit
					t.Manual[k] = true
				}
			}
			return t, d, nil
		}
	}
	return Table{}, "", fmt.Errorf("no ECB rates around %s", date)
}

func loadHist(ctx context.Context, endpoint string) error {
	histMu.Lock()
	if t, ok := histFetched[endpoint]; ok && time.Since(t) < 12*time.Hour {
		histMu.Unlock()
		return nil
	}
	histMu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("historical rates: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("historical rates: HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	if err != nil {
		return err
	}
	days := map[string]map[string]float64{}
	if strings.HasSuffix(endpoint, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil || len(zr.File) == 0 {
			return fmt.Errorf("historical rates: bad zip")
		}
		f, err := zr.File[0].Open()
		if err != nil {
			return err
		}
		defer f.Close()
		rows, err := csv.NewReader(f).ReadAll()
		if err != nil || len(rows) < 2 {
			return fmt.Errorf("historical rates: bad csv")
		}
		head := rows[0]
		for _, row := range rows[1:] {
			day := map[string]float64{}
			for i := 1; i < len(row) && i < len(head); i++ {
				if v, err := strconv.ParseFloat(strings.TrimSpace(row[i]), 64); err == nil && v > 0 {
					day[strings.TrimSpace(head[i])] = v
				}
			}
			days[strings.TrimSpace(row[0])] = day
		}
	} else {
		var doc struct {
			Cube struct {
				Days []struct {
					Time  string `xml:"time,attr"`
					Rates []struct {
						Currency string `xml:"currency,attr"`
						Rate     string `xml:"rate,attr"`
					} `xml:"Cube"`
				} `xml:"Cube"`
			} `xml:"Cube"`
		}
		if err := xml.Unmarshal(body, &doc); err != nil {
			return fmt.Errorf("historical rates unreadable: %w", err)
		}
		for _, d := range doc.Cube.Days {
			day := map[string]float64{}
			for _, r := range d.Rates {
				if v, err := strconv.ParseFloat(r.Rate, 64); err == nil && v > 0 {
					day[strings.ToUpper(r.Currency)] = v
				}
			}
			days[d.Time] = day
		}
	}
	histMu.Lock()
	for k, v := range days {
		histCache[k] = v
	}
	histFetched[endpoint] = time.Now()
	histMu.Unlock()
	return nil
}
