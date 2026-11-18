package fx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type mapSettings map[string]string

func (m mapSettings) Get(_ context.Context, k, def string) (string, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return def, nil
}
func (m mapSettings) Set(_ context.Context, _, k, v string) error { m[k] = v; return nil }

// A Saturday payment uses Friday's fixing; manual rates fill unknown currencies.
func TestTableOnUsesLastBusinessDay(t *testing.T) {
	fri := time.Now().AddDate(0, 0, -10)
	for fri.Weekday() != time.Friday {
		fri = fri.AddDate(0, 0, -1)
	}
	sat := fri.AddDate(0, 0, 1).Format("2006-01-02")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<gesmes:Envelope xmlns:gesmes="x"><Cube>
<Cube time="` + fri.Format("2006-01-02") + `"><Cube currency="USD" rate="1.10"/></Cube>
<Cube time="` + fri.AddDate(0, 0, -1).Format("2006-01-02") + `"><Cube currency="USD" rate="1.20"/></Cube>
</Cube></gesmes:Envelope>`))
	}))
	defer srv.Close()
	old := HistEndpoint
	HistEndpoint = srv.URL + "/hist.xml"
	defer func() { HistEndpoint = old }()

	tab, day, err := TableOn(context.Background(), mapSettings{KeyManual: "UAH=0.02"}, sat)
	if err != nil {
		t.Fatal(err)
	}
	if day != fri.Format("2006-01-02") {
		t.Errorf("day = %s, want Friday %s", day, fri.Format("2006-01-02"))
	}
	if v, ok := tab.Convert(1100, "USD", "EUR"); !ok || v != 1000 {
		t.Errorf("11 USD = %d EUR cents, want 1000", v)
	}
	if v, ok := tab.Convert(10000, "UAH", "EUR"); !ok || v != 200 {
		t.Errorf("100 UAH = %d EUR cents, want 200", v)
	}

	// Manual rates win over ECB: 1 USD = 0.50 EUR means 1 USD = 50 cents.
	manual, _, err := TableOn(context.Background(), mapSettings{KeyManual: "USD=0.5"}, sat)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := manual.Convert(100, "USD", "EUR"); !ok || v != 50 {
		t.Errorf("manual USD rate should win: %d", v)
	}
}

const ecbSample = `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
<Cube><Cube time="2026-11-17"><Cube currency="USD" rate="1.10"/><Cube currency="PLN" rate="4.25"/><Cube currency="BAD" rate="x"/></Cube></Cube>
</gesmes:Envelope>`

func withEndpoint(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	old := Endpoint
	Endpoint = srv.URL
	t.Cleanup(func() { Endpoint = old; srv.Close() })
}

func TestFetchRefreshLoadConvert(t *testing.T) {
	withEndpoint(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(ecbSample)) })
	ctx := context.Background()
	set := mapSettings{}
	c, err := Refresh(ctx, set, "t")
	if err != nil {
		t.Fatal(err)
	}
	if c.Date != "2026-11-17" || c.Rates["USD"] != 1.10 || c.Rates["EUR"] != 1 {
		t.Fatalf("fetched %+v", c)
	}
	if _, ok := c.Rates["BAD"]; ok {
		t.Fatal("unparseable rate must be skipped")
	}
	set[KeyManual] = "UAH=0.02"
	tab := Load(ctx, set)
	if tab.Date != "2026-11-17" || !tab.Manual["UAH"] || tab.Rates["UAH"] != 50 {
		t.Fatalf("table %+v", tab)
	}
	if got := tab.Codes(); got[0] != "EUR" || len(got) != 4 {
		t.Fatalf("codes %v", got)
	}
	if v, ok := tab.Convert(110_00, "USD", "EUR"); !ok || v != 100_00 {
		t.Fatalf("USD→EUR %d %v", v, ok)
	}
	if v, ok := tab.Convert(1000, "UAH", "eur"); !ok || v != 20 {
		t.Fatalf("UAH→EUR %d %v", v, ok)
	}
	if v, ok := tab.Convert(500, "", "USD"); !ok || v != 500 {
		t.Fatal("no source currency means no conversion")
	}
	if _, ok := tab.Convert(500, "XYZ", "EUR"); ok {
		t.Fatal("unknown currency")
	}
	// A broken manual list is ignored rather than breaking every page.
	set[KeyManual] = "garbage"
	if tab := Load(ctx, set); tab.Manual["UAH"] {
		t.Fatal("bad manual rates must be ignored")
	}
}

func TestFetchErrors(t *testing.T) {
	ctx := context.Background()
	for name, h := range map[string]http.HandlerFunc{
		"status": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) },
		"xml":    func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<not")) },
		"empty":  func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<Envelope><Cube><Cube time="x"></Cube></Cube></Envelope>`)) },
	} {
		withEndpoint(t, h)
		if _, err := Fetch(ctx); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestLoopRespectsAutoOffAndFreshRates(t *testing.T) {
	calls := 0
	withEndpoint(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(ecbSample)) })
	ctx := context.Background()
	set := &lockedSettings{m: mapSettings{KeyAuto: "off"}}
	stop := Loop(ctx, set)
	time.Sleep(50 * time.Millisecond)
	stop()
	if calls != 0 {
		t.Fatal("fx_auto=off must not fetch")
	}
	set = &lockedSettings{m: mapSettings{}}
	stop = Loop(ctx, set)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v, _ := set.Get(ctx, KeyRates, ""); v != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	if v, _ := set.Get(ctx, KeyRates, ""); v == "" {
		t.Fatal("the loop should fetch rates on start")
	}
}

type lockedSettings struct {
	mu sync.Mutex
	m  mapSettings
}

func (l *lockedSettings) Get(ctx context.Context, k, def string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Get(ctx, k, def)
}
func (l *lockedSettings) Set(ctx context.Context, a, k, v string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Set(ctx, a, k, v)
}
