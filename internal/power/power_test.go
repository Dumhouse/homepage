package power_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/power"
)

func TestReadDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rpc/Switch.GetStatus" && strings.HasPrefix(r.Host, "127"):
			w.Write([]byte(`{"id":0,"apower":21.5}`))
		case r.URL.Path == "/cm":
			w.Write([]byte(`{"StatusSNS":{"ENERGY":{"Power":33}}}`))
		case r.URL.Path == "/api/states/sensor.nas_power":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"state":"0.042","attributes":{"unit_of_measurement":"kW"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	host := strings.TrimPrefix(srv.URL, "http://")
	cfg := power.Config{HAURL: srv.URL, HAToken: "tok"}
	for _, tc := range []struct {
		src, ref string
		want     float64
	}{{power.Shelly, host, 21.5}, {power.Tasmota, srv.URL, 33}, {power.HA, "sensor.nas_power", 42}} {
		w, err := power.Read(ctx, cfg, tc.src, tc.ref)
		if err != nil || w != tc.want {
			t.Errorf("%s: %v %v, want %v", tc.src, w, err, tc.want)
		}
	}
	if _, err := power.Read(ctx, power.Config{HAURL: srv.URL, HAToken: "bad"}, power.HA, "sensor.nas_power"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("bad token: %v", err)
	}
	if _, err := power.Read(ctx, power.Config{}, power.HA, "sensor.x"); err == nil {
		t.Error("HA without settings should explain itself")
	}
}

func TestRunningMean(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	h, err := inventory.New(d).CreateHost(ctx, "test", inventory.Host{Name: "nas", Kind: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	s := power.NewStore(d)
	if err := s.Set(ctx, h.ID, power.Shelly, "10.0.0.9", 0); err != nil {
		t.Fatal(err)
	}
	for _, w := range []float64{10, 20, 30} {
		if err := s.Record(ctx, h.ID, w, nil); err != nil {
			t.Fatal(err)
		}
	}
	r, _ := s.Get(ctx, h.ID)
	if r.AvgW != 20 || r.LastW != 30 || r.Samples != 3 || r.Watts() != 20 {
		t.Fatalf("mean: %+v", r)
	}
	// Switching to a typed figure uses it and restarts the mean.
	if err := s.Set(ctx, h.ID, power.Manual, "", 45); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Get(ctx, h.ID)
	if r.Watts() != 45 || r.Samples != 0 || r.MonthlyKWh() < 32 || r.MonthlyKWh() > 33 {
		t.Fatalf("manual: %+v kwh=%v", r, r.MonthlyKWh())
	}
	if err := s.Set(ctx, h.ID, "", "", 0); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.Get(ctx, h.ID); r != nil {
		t.Fatal("empty source should remove the figure")
	}
}
