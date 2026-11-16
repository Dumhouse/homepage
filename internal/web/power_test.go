package web_test

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/integrations"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/web"
)

// Typed-in watts on a host plus a price per kWh give an Electricity row.
func TestElectricityCost(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	m := integrations.NewManager(d, integrations.NewMemorySecrets())
	m.Register(integrations.Power())
	srv, err := web.New(d, auth.None{}, web.WithIntegrations(m))
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	h, _ := inv.CreateHost(context.Background(), "test", inventory.Host{Name: "nas", Kind: "bare"})
	proj, _ := inv.CreateProject(context.Background(), "test", inventory.Project{Name: "Acme"})
	if err := inv.LinkProjectHost(context.Background(), "test", proj.ID, h.ID, "production"); err != nil {
		t.Fatal(err)
	}
	if code, loc := redirect(t, srv, "/settings/integrations/power", url.Values{"price_kwh": {"0.30"}}); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("save price: %s", loc)
	}
	hid := "/hosts/" + itoaT(h.ID)
	if code, loc := redirect(t, srv, hid+"/power", url.Values{"source": {"manual"}, "manual_w": {"40"}}); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("save power: %s", loc)
	}
	// 40 W × 730.56 h = 29.22 kWh × 0.30 = 8.77
	_, body := page(t, srv, "/costs")
	if !strings.Contains(body, "Electricity") || !strings.Contains(body, "8.77 EUR") {
		t.Fatal("costs should show the electricity row")
	}
	if strings.Count(body, "Acme") < 2 {
		t.Error("electricity row should inherit the host's project (select + by-project chart)")
	}
	_, body = page(t, srv, hid)
	if !strings.Contains(body, "40 W") || !strings.Contains(body, "8.77 EUR") {
		t.Fatal("host page should show the power card")
	}
}

func itoaT(n int64) string { return strconv.FormatInt(n, 10) }
