package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/inventory"
)

func postLocation(t *testing.T, h http.Handler, path string, form url.Values) string {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
	}
	return rec.Header().Get("Location")
}

// Deletes land somewhere sensible even when the form leaves out where it came
// from, and every page shows the ok= message.
func TestDeleteRedirectsAndFlash(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "nas", Kind: "bare"})
	a, err := inv.CreateAccess(ctx, "test", inventory.Access{TargetEntity: "host", TargetID: h.ID, Label: "web ui", Method: "password"})
	if err != nil {
		t.Fatal(err)
	}
	if loc := postLocation(t, srv, "/access/"+itoa(a.ID)+"/delete", nil); loc != "/hosts/"+itoa(h.ID)+"?ok=access-deleted" {
		t.Fatalf("access delete went to %q", loc)
	}

	d, err := inv.CreateDomain(ctx, "test", inventory.Domain{Name: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := inv.AddDomainRecord(ctx, "test", d.ID, inventory.DomainRecord{Name: "www", Type: "A", Content: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	if loc := postLocation(t, srv, "/records/"+itoa(rec.ID)+"/delete", nil); loc != "/domains?ok=record-deleted" {
		t.Fatalf("record delete went to %q", loc)
	}

	// a page that never set Flash itself (the host page) still shows the toast
	_, body := page(t, srv, "/hosts/"+itoa(h.ID)+"?ok=access-deleted")
	if !strings.Contains(body, "Login removed.") {
		t.Fatal("host page should show the flash message")
	}
}

// A health warning counts in the sidebar Alerts badge and the bell.
func TestHealthWarningInShell(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "nas", Kind: "bare"})
	if err := inv.SaveHostFacts(ctx, "test", inventory.HostFacts{HostID: h.ID, Error: "sda fails SMART", PasswordAuth: "unknown", RebootRequired: "no"}); err != nil {
		t.Fatal(err)
	}
	_, body := page(t, srv, "/hosts")
	if !strings.Contains(body, "nas needs a look") {
		t.Fatal("bell should list the health warning")
	}
}

// Starring a tile adds it to the collapsed Favourites section; starring again removes it.
func TestFavorites(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "nas", Kind: "bare", Addresses: []inventory.Address{{Kind: "lan", Address: "192.168.1.20"}}})
	if _, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "Grafana", Port: 3000, Protocol: "http"}); err != nil {
		t.Fatal(err)
	}
	_, body := page(t, srv, "/")
	if strings.Contains(body, `class="favs`) {
		t.Fatal("no favourites section before anything is starred")
	}
	svc, _ := inv.GetHost(ctx, h.ID)
	key := "service:" + itoa(svc.Services[0].ID)
	if loc := postLocation(t, srv, "/favorites", url.Values{"key": {key}}); loc != "/" {
		t.Fatalf("redirect %q", loc)
	}
	_, body = page(t, srv, "/")
	if !strings.Contains(body, `<details class="favs card"`) || strings.Contains(body, `<details class="favs card" open`) {
		t.Fatal("favourites should show, collapsed")
	}
	postLocation(t, srv, "/favorites", url.Values{"key": {key}})
	if _, body = page(t, srv, "/"); strings.Contains(body, `class="favs`) {
		t.Fatal("unstarring should remove it")
	}
	if code, _ := postForm(t, srv, "/favorites", url.Values{"key": {"service:1,host:2"}}); code != 400 {
		t.Fatalf("bad key: %d", code)
	}
}

// A location whose first hosts are filtered out (deprecated) still gets one chip, not one per host.
func TestLocationChipOnce(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	loc, _ := inv.CreateLocation(ctx, "test", inventory.Location{Name: "Lab"})
	for _, h := range []inventory.Host{
		{Name: "old1", Kind: "vm", Lifecycle: "deprecated", LocationID: &loc.ID},
		{Name: "old2", Kind: "vm", Lifecycle: "deprecated", LocationID: &loc.ID},
		{Name: "live", Kind: "vm", LocationID: &loc.ID},
	} {
		if _, err := inv.CreateHost(ctx, "test", h); err != nil {
			t.Fatal(err)
		}
	}
	_, body := page(t, srv, "/hosts")
	if n := strings.Count(body, `href="/hosts?loc=Lab"`); n != 1 {
		t.Fatalf("Lab chip appears %d times", n)
	}
}

// The "add environment" form is a normal block under the tabs (a popup inside the scrolling tab bar was clipped).
func TestAddEnvironmentForm(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	if _, err := inv.CreateProject(ctx, "test", inventory.Project{Name: "Acme", Slug: "acme"}); err != nil {
		t.Fatal(err)
	}
	_, body := page(t, srv, "/projects/acme")
	if strings.Contains(body, `id="addenv"`) || !strings.Contains(body, "addenv=1") {
		t.Fatal("form hidden until asked for")
	}
	if _, body = page(t, srv, "/projects/acme?addenv=1"); !strings.Contains(body, `id="addenv"`) {
		t.Fatal("form should show")
	}
	if loc := postLocation(t, srv, "/projects/acme/envs", url.Values{"name": {"staging"}, "base_url": {"https://s.example.com"}}); !strings.Contains(loc, "env=staging") {
		t.Fatalf("redirect %q", loc)
	}
}
