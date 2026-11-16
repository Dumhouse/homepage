package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/checks"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/web"
)

func redirect(t *testing.T, h http.Handler, path string, form url.Values) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Header().Get("Location")
}

func TestForms(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}

	// create a host, including a new location and two dynamic rows
	code, loc := redirect(t, srv, "/hosts", url.Values{
		"name": {"acme-prod"}, "hostname": {"acme.example.com"}, "kind": {"lxc"},
		"lifecycle": {"active"}, "location": {"Proxmox .250"}, "tags": {"prod, rails"},
		"addr_kind_1": {"lan"}, "addr_1": {"10.0.0.5"},
		"addr_kind_2": {"tailscale"}, "addr_2": {"100.64.0.1"},
		"ssh_user_1": {"deploy"}, "ssh_port_1": {"2222"},
	})
	if code != 303 || loc != "/hosts?ok=created" {
		t.Fatalf("create host: %d %s", code, loc)
	}
	code, body := page(t, srv, "/hosts/1")
	if code != 200 || !strings.Contains(body, "acme-prod") || !strings.Contains(body, "Proxmox .250") ||
		!strings.Contains(body, "100.64.0.1") || !strings.Contains(body, "ssh deploy@10.0.0.5 -p 2222") {
		t.Fatalf("host after create: %d", code)
	}

	// edit keeps the existing rows when the form sends them back
	code, loc = redirect(t, srv, "/hosts/1", url.Values{
		"name": {"acme-prod"}, "kind": {"lxc"}, "lifecycle": {"active"},
		"location": {"Proxmox .250"}, "description": {"edited"},
		"addr_kind_1": {"lan"}, "addr_1": {"10.0.0.5"},
		"ssh_user_1": {"deploy"}, "ssh_port_1": {"2222"},
	})
	if code != 303 || loc != "/hosts/1?ok=saved" {
		t.Fatalf("edit host: %d %s", code, loc)
	}
	_, body = page(t, srv, "/hosts/1")
	if !strings.Contains(body, "edited") || !strings.Contains(body, "10.0.0.5") || strings.Contains(body, "100.64.0.1") {
		t.Errorf("edit should keep sent rows and drop omitted ones")
	}

	// validation error renders the form again with the message
	code, body = postForm(t, srv, "/hosts", url.Values{"name": {""}})
	if code != 422 || !strings.Contains(body, "name is required") {
		t.Errorf("invalid host: %d", code)
	}

	// service add + remove
	if code, loc := redirect(t, srv, "/hosts/1/services", url.Values{"name": {"web"}, "port": {"3000"}, "protocol": {"http"}}); code != 303 || loc != "/hosts/1?ok=service" {
		t.Fatalf("add service: %d %s", code, loc)
	}
	_, body = page(t, srv, "/hosts/1")
	if !strings.Contains(body, "web") || !strings.Contains(body, "3000") {
		t.Error("service missing from host page")
	}
	if code, _ := redirect(t, srv, "/services/1/delete", url.Values{}); code != 303 {
		t.Errorf("delete service: %d", code)
	}

	// access entry
	if code, loc := redirect(t, srv, "/hosts/1/access", url.Values{
		"target_entity": {"host"}, "target_id": {"1"}, "back": {"/hosts/1"},
		"label": {"SSH"}, "method": {"ssh_key"}, "vault_item_url": {"https://vault.example.com/i/1"},
	}); code != 303 || loc != "/hosts/1?ok=access" {
		t.Fatalf("add access: %d %s", code, loc)
	}
	if _, body = page(t, srv, "/hosts/1"); !strings.Contains(body, "vault.example.com/i/1") {
		t.Error("access missing from host page")
	}

	// project create, env and app link
	if code, loc := redirect(t, srv, "/projects", url.Values{"name": {"Acme"}, "group": {"Clients"}, "status": {"active"}}); code != 303 || !strings.HasPrefix(loc, "/projects?ok=created") {
		t.Fatalf("create project: %d %s", code, loc)
	}
	if code, _ := redirect(t, srv, "/projects/acme/envs", url.Values{"name": {"production"}, "base_url": {"https://acme.example.com"}}); code != 303 {
		t.Fatalf("add env: %d", code)
	}
	if code, _ := redirect(t, srv, "/projects/acme/app-links", url.Values{
		"env": {"production"}, "label": {"Admin"}, "path": {"/admin"}, "kind": {"admin"},
	}); code != 303 {
		t.Fatalf("add app link: %d", code)
	}
	if code, _ := redirect(t, srv, "/projects/acme/access", url.Values{
		"target_entity": {"project"}, "target_id": {"1"}, "back": {"/projects/acme"},
		"label": {"Admin"}, "method": {"password"},
	}); code != 303 {
		t.Fatalf("add project access: %d", code)
	}
	_, body = page(t, srv, "/projects/acme")
	if !strings.Contains(body, "https://acme.example.com/admin") || !strings.Contains(body, "Clients") {
		t.Error("project page missing app link or group")
	}

	// project edit with a slug change redirects to the new slug
	if code, loc := redirect(t, srv, "/projects/acme", url.Values{
		"name": {"Acme"}, "slug": {"acme-app"}, "group": {"Clients"}, "status": {"active"},
	}); code != 303 || loc != "/projects/acme-app?ok=saved" {
		t.Fatalf("edit project: %d %s", code, loc)
	}

	// app link and access deletion, with local-only back targets
	if code, loc := redirect(t, srv, "/app-links/1/delete", url.Values{"back": {"/projects/acme?env=production"}}); code != 303 || loc != "/projects/acme?env=production" {
		t.Fatalf("delete app link: %d %s", code, loc)
	}
	if _, body = page(t, srv, "/projects/acme"); strings.Contains(body, "https://acme.example.com/admin") {
		t.Error("deleted app link still rendered")
	}
	if code, loc := redirect(t, srv, "/access/2/delete", url.Values{"back": {"https://evil.example"}}); code != 303 || loc != "/" {
		t.Errorf("open redirect should be refused: %d %s", code, loc)
	}
	if _, body = page(t, srv, "/projects/acme"); strings.Contains(body, ">Admin<") {
		t.Error("deleted project access still rendered")
	}
	if code, _ := redirect(t, srv, "/access/1/delete", url.Values{"back": {"/hosts/1"}}); code != 303 {
		t.Errorf("delete host access: %d", code)
	}

	// delete + undo banner + restore
	if code, loc := redirect(t, srv, "/projects/acme-app/delete", url.Values{}); code != 303 || !strings.Contains(loc, "ok=deleted") {
		t.Fatalf("delete project: %d %s", code, loc)
	}
	_, body = page(t, srv, "/projects?ok=deleted&id=1&name=Acme")
	if !strings.Contains(body, `/projects/1/restore`) || !strings.Contains(body, "Undo") {
		t.Error("undo banner missing")
	}
	if code, _ := redirect(t, srv, "/projects/1/restore", url.Values{}); code != 303 {
		t.Errorf("restore project: %d", code)
	}

	if code, _ := redirect(t, srv, "/hosts/1/delete", url.Values{}); code != 303 {
		t.Fatalf("delete host: %d", code)
	}
	_, body = page(t, srv, "/hosts?ok=deleted&id=1&name=acme-prod")
	if !strings.Contains(body, `/hosts/1/restore`) {
		t.Error("host undo banner missing")
	}
	if code, _ := redirect(t, srv, "/hosts/1/restore", url.Values{}); code != 303 {
		t.Errorf("restore host: %d", code)
	}
}

func TestExportsAndProtoPreview(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	if code, loc := redirect(t, srv, "/hosts", url.Values{
		"name": {"acme"}, "addr_1": {"10.0.0.5"}, "ssh_user_1": {"deploy"}, "ssh_port_1": {"2222"},
	}); code != 303 {
		t.Fatal("seed host", code, loc)
	}
	code, body := page(t, srv, "/export/ssh-config")
	if code != 200 || !strings.Contains(body, "Host acme") || !strings.Contains(body, "User deploy") {
		t.Errorf("ssh config: %d %s", code, body)
	}
	code, body = page(t, srv, "/export/inventory.json")
	if code != 200 || !strings.Contains(body, `"acme"`) {
		t.Errorf("json export: %d", code)
	}
	// prototype is still available as a preview, but no longer at the old URLs
	if code, _ := page(t, srv, "/design/dashboard.html"); code != 200 {
		t.Errorf("design preview: %d", code)
	}
	if code, _ := page(t, srv, "/dashboard.html"); code != 404 {
		t.Errorf("old prototype URL should be gone: %d", code)
	}
}

func TestDomainPages(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	if code, loc := redirect(t, srv, "/domains", url.Values{
		"name": {"example.com"}, "registrar": {"OVH"}, "account": {"billing@example.com"},
		"expires_at": {"2026-12-01"}, "price": {"12.99"}, "currency": {"EUR"},
		"auto_renew": {"on"}, "source": {"manual"}, "payment_label": {"Revolut"},
		"dns": {"1"}, "nameservers": {"ns1.example.net, ns2.example.net"}, "dnssec": {"on"}, "transfer_lock": {"on"},
	}); code != 303 || loc != "/domains?ok=created" {
		t.Fatalf("create domain: %d %s", code, loc)
	}
	code, body := page(t, srv, "/domains")
	if code != 200 || !strings.Contains(body, "example.com") || !strings.Contains(body, "12.99 EUR") {
		t.Fatalf("domains list: %d", code)
	}
	code, body = page(t, srv, "/domains/1")
	if code != 200 || !strings.Contains(body, "Registration") || !strings.Contains(body, "OVH") {
		t.Fatalf("domain page: %d", code)
	}
	if !strings.Contains(body, "ns1.example.net") || !strings.Contains(body, "ns2.example.net") ||
		!strings.Contains(body, "signed") || !strings.Contains(body, "locked") {
		t.Error("nameservers/DNSSEC/transfer lock missing from the domain page")
	}
	if code, _ := redirect(t, srv, "/domains/1/records", url.Values{"name": {"www"}, "type": {"A"}, "content": {"10.0.0.5"}}); code != 303 {
		t.Fatalf("add record: %d", code)
	}
	_, body = page(t, srv, "/domains/1")
	if !strings.Contains(body, "www") || !strings.Contains(body, "10.0.0.5") {
		t.Error("record missing from domain page")
	}
	if code, loc := redirect(t, srv, "/domains/1/delete", nil); code != 303 || !strings.Contains(loc, "ok=deleted") {
		t.Fatalf("delete domain: %d %s", code, loc)
	}
	_, body = page(t, srv, "/domains?ok=deleted&id=1&name=example.com")
	if !strings.Contains(body, `/domains/1/restore`) {
		t.Error("undo banner missing")
	}
	if code, _ := redirect(t, srv, "/domains/1/restore", nil); code != 303 {
		t.Errorf("restore: %d", code)
	}
}

func TestCalendarPage(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := redirect(t, srv, "/domains", url.Values{
		"name": {"soon.example"}, "expires_at": {"2026-11-25"}, "auto_renew": {"on"},
	}); code != 303 {
		t.Fatal("seed domain", code)
	}
	if code, _ := redirect(t, srv, "/maintenance", url.Values{
		"target_entity": {""}, "duration_min": {"60"}, "reason": {"upgrade night"},
	}); code != 303 {
		t.Fatal("seed maintenance", code)
	}
	code, body := page(t, srv, "/calendar")
	if code != 200 || !strings.Contains(body, "soon.example") || !strings.Contains(body, "upgrade night") || !strings.Contains(body, "Domain renewals") {
		t.Errorf("calendar: %d", code)
	}
}

func TestCostsPage(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := redirect(t, srv, "/domains", url.Values{"name": {"paid.example"}, "price": {"12"}, "expires_at": {"2026-12-01"}}); code != 303 {
		t.Fatal("seed domain")
	}
	if code, _ := redirect(t, srv, "/costs", url.Values{
		"label": {"Hetzner"}, "amount": {"5"}, "currency": {"EUR"}, "cycle": {"monthly"}, "auto_renew": {"on"},
	}); code != 303 {
		t.Fatalf("add cost: %d", code)
	}
	code, body := page(t, srv, "/costs")
	if code != 200 || !strings.Contains(body, "Hetzner") || !strings.Contains(body, "paid.example") {
		t.Fatalf("costs page: %d", code)
	}
	if !strings.Contains(body, "6.00 EUR") || !strings.Contains(body, "72.00 EUR") {
		t.Errorf("totals should include monthly + yearly/12 and domain: %s", body[:0])
	}
	code, body = page(t, srv, "/costs.csv")
	if code != 200 || !strings.Contains(body, "Hetzner") || !strings.Contains(body, "paid.example") {
		t.Errorf("csv: %d %s", code, body)
	}
	// delete the explicit cost through the undo flow
	if code, _ := redirect(t, srv, "/costs/1/delete", nil); code != 303 {
		t.Fatalf("delete cost: %d", code)
	}
	_, body = page(t, srv, "/costs?ok=deleted&id=1&name=Hetzner")
	if !strings.Contains(body, `/costs/1/restore`) {
		t.Error("undo banner missing")
	}
	if code, _ := redirect(t, srv, "/costs/1/restore", nil); code != 303 {
		t.Errorf("restore cost: %d", code)
	}
}

func TestEditForms(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := redirect(t, srv, "/hosts", url.Values{"name": {"acme"}, "addr_1": {"10.0.0.5"}}); code != 303 {
		t.Fatal("seed host")
	}
	if code, _ := redirect(t, srv, "/hosts/1/services", url.Values{"name": {"web"}, "port": {"3000"}, "protocol": {"http"}}); code != 303 {
		t.Fatal("seed service")
	}

	code, body := page(t, srv, "/services/1/edit")
	if code != 200 || !strings.Contains(body, `value="web"`) {
		t.Fatalf("service form: %d", code)
	}
	if code, loc := redirect(t, srv, "/services/1", url.Values{"name": {"api"}, "port": {"4000"}, "protocol": {"https"}}); code != 303 || loc != "/hosts/1?ok=saved" {
		t.Fatalf("save service: %d %s", code, loc)
	}
	_, body = page(t, srv, "/hosts/1")
	if !strings.Contains(body, "api") || !strings.Contains(body, "4000") {
		t.Error("service edit not reflected")
	}

	if code, _ := redirect(t, srv, "/hosts/1/access", url.Values{
		"target_entity": {"host"}, "target_id": {"1"}, "back": {"/hosts/1"},
		"label": {"SSH"}, "method": {"ssh_key"},
	}); code != 303 {
		t.Fatal("seed access")
	}
	code, body = page(t, srv, "/access/1/edit")
	if code != 200 || !strings.Contains(body, `value="SSH"`) {
		t.Fatalf("access form: %d", code)
	}
	if code, loc := redirect(t, srv, "/access/1", url.Values{"label": {"SSH root"}, "method": {"password"}, "account": {"root"}, "twofa": {"totp"}}); code != 303 || loc != "/hosts/1?ok=saved" {
		t.Fatalf("save access: %d %s", code, loc)
	}
	_, body = page(t, srv, "/hosts/1")
	if !strings.Contains(body, "SSH root") || !strings.Contains(body, "totp") {
		t.Error("access edit not reflected")
	}

	if code, _ := redirect(t, srv, "/projects", url.Values{"name": {"Acme"}}); code != 303 {
		t.Fatal("seed project")
	}
	if code, _ := redirect(t, srv, "/projects/acme/envs", url.Values{"name": {"production"}, "base_url": {"https://acme.example.com"}}); code != 303 {
		t.Fatal("seed env")
	}
	if code, _ := redirect(t, srv, "/projects/acme/app-links", url.Values{"env": {"production"}, "label": {"Admin"}, "path": {"/admin"}, "kind": {"admin"}}); code != 303 {
		t.Fatal("seed app link")
	}
	code, body = page(t, srv, "/app-links/1/edit")
	if code != 200 || !strings.Contains(body, `value="Admin"`) {
		t.Fatalf("app link form: %d", code)
	}
	if code, _ := redirect(t, srv, "/app-links/1", url.Values{"label": {"Sidekiq"}, "path": {"/sidekiq"}, "kind": {"jobs"}}); code != 303 {
		t.Fatalf("save app link: %d", code)
	}
	_, body = page(t, srv, "/projects/acme")
	if !strings.Contains(body, "Sidekiq") || !strings.Contains(body, "https://acme.example.com/sidekiq") {
		t.Error("app link edit not reflected")
	}
}

func TestCalendarShowsCertificates(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	ctx := context.Background()
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "edge"})
	cs := checks.New(d)
	c, err := cs.Create(ctx, "test", checks.Check{TargetEntity: "host", TargetID: h.ID, Kind: "tls", Params: map[string]any{"address": "edge.example.com:443"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Record(ctx, *c, checks.Result{OK: true, CertDays: 12, CertIssuer: "Let's Encrypt"}); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	code, body := page(t, srv, "/calendar")
	if code != 200 || !strings.Contains(body, "edge") || !strings.Contains(body, "12 days") || !strings.Contains(body, "Let&#39;s Encrypt") {
		t.Errorf("certificate missing from calendar: %d", code)
	}
}

func TestLocationPages(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	if code, loc := redirect(t, srv, "/locations", url.Values{
		"name": {"Proxmox .250"}, "kind": {"proxmox"}, "url": {"https://192.168.1.250:8006"},
	}); code != 303 || loc != "/locations?ok=created" {
		t.Fatalf("create location: %d %s", code, loc)
	}
	code, body := page(t, srv, "/locations")
	if code != 200 || !strings.Contains(body, "Proxmox .250") || !strings.Contains(body, "0 hosts") {
		t.Fatalf("locations list: %d", code)
	}
	locs, _ := inv.ListLocations(ctx)
	if len(locs) != 1 {
		t.Fatalf("locations: %+v", locs)
	}
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "acme", LocationID: &locs[0].ID})
	_, body = page(t, srv, "/locations")
	if !strings.Contains(body, "1 host") || !strings.Contains(body, "/hosts?loc=Proxmox&#43;.250") {
		t.Errorf("host count/filter link missing")
	}
	if code, filtered := page(t, srv, "/hosts?loc=Proxmox+.250"); code != 200 || !strings.Contains(filtered, "acme") {
		t.Errorf("location filter should list the host: %d", code)
	}
	if code, other := page(t, srv, "/hosts?loc=Elsewhere"); code != 200 || strings.Contains(other, "acme") {
		t.Errorf("location filter should hide other hosts: %d", code)
	}
	code, loc := redirect(t, srv, "/locations/"+itoaT(locs[0].ID)+"/delete", nil)
	if code != 303 || !strings.Contains(loc, "ok=err") || !strings.Contains(loc, "move+them+first") {
		t.Errorf("delete in use should fail with a hint: %d %s", code, loc)
	}
	if code, loc := redirect(t, srv, "/locations/"+itoaT(locs[0].ID), url.Values{"name": {"Lab"}, "kind": {"bare-metal"}}); code != 303 || loc != "/locations?ok=saved" {
		t.Fatalf("edit location: %d %s", code, loc)
	}
	h.LocationID = nil
	if _, err := inv.UpdateHost(ctx, "test", h.ID, *h); err != nil {
		t.Fatal(err)
	}
	if code, _ := redirect(t, srv, "/locations/"+itoaT(locs[0].ID)+"/delete", nil); code != 303 {
		t.Fatalf("delete empty location: %d", code)
	}
	if _, body = page(t, srv, "/locations?ok=deleted&id=1&name=Lab"); !strings.Contains(body, "/locations/1/restore") {
		t.Error("undo banner missing")
	}
	if code, _ := redirect(t, srv, "/locations/1/restore", nil); code != 303 {
		t.Errorf("restore location: %d", code)
	}
}
