package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/discovery"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/rdap"
	"github.com/Dumhouse/homepage/internal/web"
)

func newTestServer(t *testing.T) (http.Handler, *inventory.Store) {
	t.Helper()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	return srv, inventory.New(d)
}

// Open SSH must be a real ssh:// link (html/template rewrites unknown schemes
// to #ZgotmplZ), with the port; the page has real tabs and the AI drawer.
func TestHostPageSSHLinkTabsAndContext(t *testing.T) {
	srv, inv := newTestServer(t)
	h, err := inv.CreateHost(context.Background(), "test", inventory.Host{
		Name: "acme", Addresses: []inventory.Address{{Address: "10.0.0.5"}},
		SSH: []inventory.SSHAccess{{User: "deploy", Port: 2222}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, body := page(t, srv, "/hosts/"+itoa(h.ID))
	if strings.Contains(body, "ZgotmplZ") || !strings.Contains(body, `href="ssh://deploy@10.0.0.5:2222"`) {
		t.Fatal("Open SSH link is broken")
	}
	if !strings.Contains(body, `data-tab-link="facts"`) || !strings.Contains(body, `data-tab="runbook"`) {
		t.Fatal("tabs should switch content")
	}
	if !strings.Contains(body, `data-async`) {
		t.Fatal("Check now should submit in the background")
	}
	code, md := page(t, srv, "/hosts/"+itoa(h.ID)+"/context.md")
	if code != 200 || !strings.Contains(md, "acme") {
		t.Fatalf("context: %d %q", code, md)
	}
	_, hosts := page(t, srv, "/hosts")
	if strings.Contains(hosts, "ZgotmplZ") {
		t.Fatal("hosts list SSH link is broken")
	}
}

// Facts can be typed in by hand.
func TestFactsEditByHand(t *testing.T) {
	srv, inv := newTestServer(t)
	h, _ := inv.CreateHost(context.Background(), "test", inventory.Host{Name: "box"})
	code, loc := redirect(t, srv, "/hosts/"+itoa(h.ID)+"/facts/edit", url.Values{
		"os": {"Debian 12"}, "disk_used_pct": {"250"}, "reboot_required": {"yes"}, "password_auth": {"evil"},
	})
	if code != 303 || !strings.Contains(loc, "ok=facts-saved") {
		t.Fatalf("save: %d %s", code, loc)
	}
	f, err := inv.GetHostFacts(context.Background(), h.ID)
	if err != nil || f.OS != "Debian 12" || f.DiskUsedPct != 100 || f.RebootRequired != "yes" || f.PasswordAuth != "unknown" {
		t.Fatalf("facts: %+v %v", f, err)
	}
}

// Check now with nothing to check explains why instead of hanging.
func TestHostCheckNowNothingToCheck(t *testing.T) {
	srv, inv := newTestServer(t)
	h, _ := inv.CreateHost(context.Background(), "test", inventory.Host{Name: "empty"})
	code, loc := redirect(t, srv, "/hosts/"+itoa(h.ID)+"/check", nil)
	if code != 303 || !strings.Contains(loc, "ok=err") {
		t.Fatalf("check: %d %s", code, loc)
	}
}

// Editing a domain keeps imported fields that are no longer on the form,
// and the RDAP lookup fills registrar and expiry.
func TestDomainEditKeepsHiddenFieldsAndLookup(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	d, err := inv.CreateDomain(ctx, "test", inventory.Domain{Name: "example.com", Nameservers: "ns1.cf.com", DNSSEC: true, Source: "cloudflare"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := redirect(t, srv, "/domains/"+itoa(d.ID), url.Values{"name": {"example.com"}, "registrar": {"OVH"}}); code != 303 {
		t.Fatalf("save: %d", code)
	}
	got, _ := inv.GetDomain(ctx, d.ID)
	if got.Nameservers != "ns1.cf.com" || !got.DNSSEC || got.Source != "cloudflare" || got.Registrar != "OVH" {
		t.Fatalf("hidden fields lost: %+v", got)
	}

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"events":[{"eventAction":"expiration","eventDate":"2027-03-04T10:00:00Z"}],
		  "entities":[{"roles":["registrar"],"vcardArray":["vcard",[["version",{},"text","4.0"],["fn",{},"text","Cloudflare, Inc."]]]}]}`))
	}))
	defer fake.Close()
	old := rdap.Endpoint
	rdap.Endpoint = fake.URL + "/domain/"
	defer func() { rdap.Endpoint = old }()
	if code, loc := redirect(t, srv, "/domains/"+itoa(d.ID)+"/lookup", nil); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("lookup: %d %s", code, loc)
	}
	got, _ = inv.GetDomain(ctx, d.ID)
	if got.Registrar != "Cloudflare, Inc." || got.ExpiresAt != "2027-03-04" {
		t.Fatalf("lookup result: %+v", got)
	}
}

// The scanned ports are editable and shown on the page.
func TestDiscoveryPortsSetting(t *testing.T) {
	srv, _ := newTestServer(t)
	if code, _ := postForm(t, srv, "/discovery/subnets", url.Values{"ports": {"22, 8000-8002,abc"}}); code != 303 {
		t.Fatalf("save: %d", code)
	}
	_, body := page(t, srv, "/discovery")
	if !strings.Contains(body, "22, 8000, 8001, 8002") {
		t.Fatal("custom ports should be listed")
	}
	if got := discovery.ParsePorts("443 22 22 70000 5-3"); len(got) != 2 || got[0] != 22 {
		t.Fatalf("ParsePorts: %v", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
