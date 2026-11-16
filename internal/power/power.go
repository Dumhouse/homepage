// Package power tracks what each host draws from the wall: a typed-in
// figure, or live readings from Home Assistant, a Shelly or a Tasmota plug.
// The Costs page turns the average into a monthly electricity cost.
package power

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	Manual  = "manual"
	HA      = "ha"
	Shelly  = "shelly"
	Tasmota = "tasmota"
	// window is how many 5-minute readings the running mean covers (24 h).
	window = 288
)

// Reading is a host's power figure.
type Reading struct {
	HostID    int64   `json:"host_id"`
	Source    string  `json:"source"`
	Ref       string  `json:"ref,omitempty"` // HA entity id or plug address
	ManualW   float64 `json:"manual_w,omitempty"`
	LastW     float64 `json:"last_w,omitempty"`
	AvgW      float64 `json:"avg_w,omitempty"`
	Samples   int     `json:"samples,omitempty"`
	UpdatedAt string  `json:"updated_at,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// Watts is the figure costs use: the measured average, else the typed one.
func (r Reading) Watts() float64 {
	if r.Source != Manual && r.Samples > 0 {
		return r.AvgW
	}
	return r.ManualW
}

// MonthlyKWh is the energy at Watts() over an average month.
func (r Reading) MonthlyKWh() float64 { return r.Watts() * 24 * 30.44 / 1000 }

// Live reports a reading that comes from a device rather than a guess.
func (r Reading) Live() bool { return r.Source != Manual && r.Source != "" }

// Config is what the power integration stores: price and Home Assistant.
type Config struct {
	PricePerKWh float64 // in the default currency
	HAURL       string
	HAToken     string
}

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

const cols = `host_id,source,ref,manual_w,last_w,avg_w,samples,COALESCE(updated_at,''),error`

func scan(s interface{ Scan(...any) error }) (Reading, error) {
	var r Reading
	err := s.Scan(&r.HostID, &r.Source, &r.Ref, &r.ManualW, &r.LastW, &r.AvgW, &r.Samples, &r.UpdatedAt, &r.Error)
	return r, err
}

// Get returns the host's reading, or nil when it has none.
func (s *Store) Get(ctx context.Context, hostID int64) (*Reading, error) {
	r, err := scan(s.db.QueryRowContext(ctx, `SELECT `+cols+` FROM host_power WHERE host_id=?`, hostID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// List returns every host with a power figure, keyed by host id.
func (s *Store) List(ctx context.Context) (map[int64]Reading, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cols+` FROM host_power`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Reading{}
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out[r.HostID] = r
	}
	return out, rows.Err()
}

// Set stores how a host's power is known. Changing the source or ref
// restarts the average. An empty source removes the row.
func (s *Store) Set(ctx context.Context, hostID int64, source, ref string, manualW float64) error {
	source, ref = strings.TrimSpace(source), strings.TrimSpace(ref)
	if source == "" || (source == Manual && manualW <= 0) {
		_, err := s.db.ExecContext(ctx, `DELETE FROM host_power WHERE host_id=?`, hostID)
		return err
	}
	switch source {
	case Manual, HA, Shelly, Tasmota:
	default:
		return fmt.Errorf("unknown power source %q", source)
	}
	if source != Manual && ref == "" {
		return fmt.Errorf("give the %s entity or plug address", source)
	}
	if manualW < 0 || manualW > 100000 {
		return fmt.Errorf("watts must be between 0 and 100000")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO host_power(host_id,source,ref,manual_w) VALUES(?,?,?,?)
		ON CONFLICT(host_id) DO UPDATE SET
		  samples=CASE WHEN host_power.source=excluded.source AND host_power.ref=excluded.ref THEN host_power.samples ELSE 0 END,
		  avg_w=CASE WHEN host_power.source=excluded.source AND host_power.ref=excluded.ref THEN host_power.avg_w ELSE 0 END,
		  error=CASE WHEN host_power.source=excluded.source AND host_power.ref=excluded.ref THEN host_power.error ELSE '' END,
		  source=excluded.source, ref=excluded.ref, manual_w=excluded.manual_w`,
		hostID, source, ref, manualW)
	return err
}

// Record adds one reading (or an error) to the running mean.
func (s *Store) Record(ctx context.Context, hostID int64, w float64, readErr error) error {
	if readErr != nil {
		_, err := s.db.ExecContext(ctx, `UPDATE host_power SET error=?, updated_at=datetime('now') WHERE host_id=?`, readErr.Error(), hostID)
		return err
	}
	r, err := s.Get(ctx, hostID)
	if err != nil || r == nil {
		return err
	}
	n := min(r.Samples+1, window)
	avg := r.AvgW + (w-r.AvgW)/float64(n)
	_, err = s.db.ExecContext(ctx, `UPDATE host_power SET last_w=?, avg_w=?, samples=?, error='', updated_at=datetime('now') WHERE host_id=?`,
		w, math.Round(avg*10)/10, n, hostID)
	return err
}

var client = &http.Client{Timeout: 8 * time.Second}

// Read asks the device for the current draw in watts.
func Read(ctx context.Context, cfg Config, source, ref string) (float64, error) {
	switch source {
	case HA:
		if cfg.HAURL == "" || cfg.HAToken == "" {
			return 0, errors.New("Home Assistant isn't set up — add its URL and a long-lived token in Settings → Integrations → Electricity")
		}
		var st struct {
			State      string         `json:"state"`
			Attributes map[string]any `json:"attributes"`
		}
		u := strings.TrimRight(cfg.HAURL, "/") + "/api/states/" + url.PathEscape(ref)
		if err := getJSON(ctx, u, cfg.HAToken, &st); err != nil {
			return 0, err
		}
		w, err := strconv.ParseFloat(st.State, 64)
		if err != nil {
			return 0, fmt.Errorf("%s is %q, not a number of watts", ref, st.State)
		}
		if unit, _ := st.Attributes["unit_of_measurement"].(string); unit == "kW" {
			w *= 1000
		}
		return w, nil
	case Shelly:
		base := plugURL(ref)
		var gen2 struct {
			APower *float64 `json:"apower"`
		}
		if err := getJSON(ctx, base+"/rpc/Switch.GetStatus?id=0", "", &gen2); err == nil && gen2.APower != nil {
			return *gen2.APower, nil
		}
		var gen1 struct {
			Meters []struct {
				Power float64 `json:"power"`
			} `json:"meters"`
		}
		if err := getJSON(ctx, base+"/status", "", &gen1); err != nil {
			return 0, err
		}
		if len(gen1.Meters) == 0 {
			return 0, errors.New("this Shelly has no power meter")
		}
		return gen1.Meters[0].Power, nil
	case Tasmota:
		var st struct {
			StatusSNS struct {
				Energy *struct {
					Power float64 `json:"Power"`
				} `json:"ENERGY"`
			} `json:"StatusSNS"`
		}
		if err := getJSON(ctx, plugURL(ref)+"/cm?cmnd=Status%208", "", &st); err != nil {
			return 0, err
		}
		if st.StatusSNS.Energy == nil {
			return 0, errors.New("this Tasmota device reports no ENERGY readings")
		}
		return st.StatusSNS.Energy.Power, nil
	}
	return 0, fmt.Errorf("nothing to read for source %q", source)
}

func plugURL(ref string) string {
	if !strings.Contains(ref, "://") {
		ref = "http://" + ref
	}
	return strings.TrimRight(ref, "/")
}

func getJSON(ctx context.Context, u, bearer string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return errors.New("HTTP 401 — the token was refused")
	case res.StatusCode == http.StatusNotFound:
		return errors.New("HTTP 404 — no such entity or endpoint")
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(v)
}

// PollOnce reads every live source and records the result.
func (s *Store) PollOnce(ctx context.Context, cfg Config) {
	all, err := s.List(ctx)
	if err != nil {
		return
	}
	for id, r := range all {
		if !r.Live() {
			continue
		}
		w, err := Read(ctx, cfg, r.Source, r.Ref)
		_ = s.Record(ctx, id, w, err)
	}
}

// Loop polls every 5 minutes until ctx ends. cfg is read each round so
// settings changes apply without a restart.
func (s *Store) Loop(ctx context.Context, cfg func(context.Context) Config) func() {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			s.PollOnce(ctx, cfg(ctx))
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return cancel
}
