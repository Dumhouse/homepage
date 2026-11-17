package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/inventory"
)

// Exports open in the browser; a host warning from facts shows on Alerts and
// the dashboard.
func TestExportsAndHealthWarnings(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "nas", Kind: "bare", Addresses: []inventory.Address{{Kind: "lan", Address: "192.168.1.20"}}})
	if _, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "Grafana", Port: 3000, Protocol: "http"}); err != nil {
		t.Fatal(err)
	}
	code, body := page(t, srv, "/export/homepage.yaml")
	if code != 200 || !strings.Contains(body, `href: "http://192.168.1.20:3000"`) {
		t.Fatalf("homepage: %d %s", code, body)
	}
	if code, _ := page(t, srv, "/export/nope.txt"); code != 404 {
		t.Fatalf("unknown export: %d", code)
	}
	// the token route refuses anonymous cross-origin callers
	req := httptest.NewRequest("GET", "/api/v1/export/ansible.yaml", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusOK {
		t.Fatalf("api export: %d", rec.Code)
	}

	if err := inv.SaveHostFacts(ctx, "test", inventory.HostFacts{HostID: h.ID, Error: "pool tank is DEGRADED", PasswordAuth: "unknown", RebootRequired: "no"}); err != nil {
		t.Fatal(err)
	}
	_, body = page(t, srv, "/problems")
	if !strings.Contains(body, "pool tank is DEGRADED") || !strings.Contains(body, "nas needs a look") {
		t.Fatal("alerts should list the facts warning")
	}
	_, body = page(t, srv, "/?view=launcher")
	if !strings.Contains(body, "pool tank is DEGRADED") {
		t.Fatal("dashboard should list the facts warning")
	}
}
