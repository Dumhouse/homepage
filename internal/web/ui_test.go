package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/web"
)

func page(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func postForm(t *testing.T, h http.Handler, path string, form url.Values) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestPages(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	ctx := context.Background()
	loc, _ := inv.CreateLocation(ctx, "test", inventory.Location{Name: "Lab"})
	hst, err := inv.CreateHost(ctx, "test", inventory.Host{
		Name: "acme-prod", Hostname: "acme.example.com", LocationID: &loc.ID, Tags: []string{"prod"},
		Addresses: []inventory.Address{{Address: "10.0.0.5"}},
		SSH:       []inventory.SSHAccess{{User: "deploy", Port: 2222, Status: "password_allowed"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inv.AddService(ctx, "test", hst.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http", Domain: "acme.example.com"}); err != nil {
		t.Fatal(err)
	}
	proj, err := inv.CreateProject(ctx, "test", inventory.Project{Name: "Acme", Group: "Clients", Runbook: "# Deploy\n\nRun the pipeline."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inv.AddProjectEnv(ctx, "test", proj.ID, inventory.ProjectEnv{Name: "production", BaseURL: "https://acme.example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.AddAppLink(ctx, "test", proj.ID, inventory.AppLink{Label: "Admin", Path: "/admin", Kind: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := inv.LinkProjectHost(ctx, "test", proj.ID, hst.ID, "production"); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.CreateAccess(ctx, "test", inventory.Access{
		TargetEntity: "host", TargetID: hst.ID, Label: "SSH", Method: "ssh_key",
		VaultItemURL: "https://vault.example.com/i/1",
	}); err != nil {
		t.Fatal(err)
	}

	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}

	code, body := page(t, srv, "/")
	if code != 200 || !strings.Contains(body, "Home Services") || !strings.Contains(body, "acme-prod") {
		t.Fatalf("home: %d", code)
	}
	if !strings.Contains(body, "Infrastructure") || !strings.Contains(body, "http://acme.example.com") {
		t.Error("tiles should include hosts and service URLs")
	}
	code, body = page(t, srv, "/?group=Clients")
	if code != 200 || !strings.Contains(body, "Acme") || strings.Contains(body, "acme-prod") {
		t.Errorf("group filter: %d", code)
	}
	if c, _ := postForm(t, srv, "/prefs", url.Values{"tile": {"site"}}); c != http.StatusSeeOther {
		t.Fatalf("POST /prefs tile: %d", c)
	}
	code, body = page(t, srv, "/")
	if code != 200 || !strings.Contains(body, "https://acme.example.com") {
		t.Errorf("tile click=site should use the env base URL: %d", code)
	}
	postForm(t, srv, "/prefs", url.Values{"view": {"launcher"}})
	code, body = page(t, srv, "/?view=launcher") // ?view= alone no longer saves anything
	if !strings.Contains(body, "launcherGrid") {
		t.Fatal("launcher choice was not saved by POST /prefs")
	}
	code, body = page(t, srv, "/")
	if code != 200 || !strings.Contains(body, "launcherGrid") || !strings.Contains(body, "Acme") {
		t.Fatalf("launcher: %d", code)
	}
	_, body = page(t, srv, "/")
	if !strings.Contains(body, "launcherGrid") {
		t.Error("home view should be remembered")
	}
	// back to tiles for the remaining checks
	_, body = page(t, srv, "/?view=tiles")
	code, body = page(t, srv, "/hosts")
	if code != 200 || !strings.Contains(body, "acme-prod") || !strings.Contains(body, "Lab") {
		t.Fatalf("hosts: %d", code)
	}
	code, body = page(t, srv, "/hosts?q=10.0.0.5")
	if code != 200 || !strings.Contains(body, "acme-prod") {
		t.Fatalf("hosts search: %d", code)
	}
	code, body = page(t, srv, "/hosts?q=nothing")
	if code != 200 || strings.Contains(body, "acme-prod") {
		t.Fatalf("hosts filter: %d", code)
	}
	code, body = page(t, srv, "/hosts/1")
	if code != 200 || !strings.Contains(body, "10.0.0.5") || !strings.Contains(body, "ssh deploy@10.0.0.5 -p 2222") {
		t.Fatalf("host page: %d", code)
	}
	if !strings.Contains(body, "https://vault.example.com/i/1") {
		t.Error("host access link missing")
	}
	if !strings.Contains(body, "accepts SSH passwords") || !strings.Contains(body, "PasswordAuthentication no") {
		t.Error("ssh fix guide missing for a password-allowed host")
	}
	code, body = page(t, srv, "/hosts/999")
	if code != 404 {
		t.Errorf("missing host: %d", code)
	}

	code, body = page(t, srv, "/projects")
	if code != 200 || !strings.Contains(body, "Acme") || !strings.Contains(body, "Clients") {
		t.Fatalf("projects: %d", code)
	}
	code, body = page(t, srv, "/projects/acme")
	if code != 200 || !strings.Contains(body, "https://acme.example.com/admin") || !strings.Contains(body, "Deploy") {
		t.Fatalf("project page: %d", code)
	}
	if !strings.Contains(body, "/hosts/1") {
		t.Error("project host link missing")
	}
	code, body = page(t, srv, "/projects/acme?env=staging")
	if code != 200 || !strings.Contains(body, "production") {
		t.Fatalf("unknown env should fall back to tabs: %d", code)
	}

	code, body = page(t, srv, "/settings/mcp")
	if code != 200 || !strings.Contains(body, "MCP &amp; tokens") || !strings.Contains(body, "New token") {
		t.Fatalf("settings mcp: %d", code)
	}
	code, body = postForm(t, srv, "/settings/mcp/tokens", url.Values{"name": {"claude-code"}, "scope": {"write"}})
	if code != 200 || !strings.Contains(body, "hs_") || !strings.Contains(body, "claude-code") {
		t.Fatalf("create token: %d", code)
	}
	newToken := ""
	for _, f := range strings.Fields(body) {
		if strings.HasPrefix(f, "hs_") {
			newToken = strings.Trim(f, "<\"'")
			break
		}
	}
	if newToken == "" {
		t.Fatal("token plaintext not shown once")
	}
	code, body = page(t, srv, "/settings/mcp")
	if code != 200 || !strings.Contains(body, "claude-code") || strings.Contains(body, newToken) {
		t.Fatalf("token list should not repeat the secret: %d", code)
	}
	code, _ = postForm(t, srv, "/settings/mcp/tokens/revoke", url.Values{"name": {"claude-code"}})
	if code != 303 {
		t.Fatalf("revoke: %d", code)
	}
	_, body = page(t, srv, "/settings/mcp?ok=revoked")
	if !strings.Contains(body, "revoked") || !strings.Contains(body, "Token revoked.") {
		t.Error("revoked state or flash missing")
	}
	code, body = postForm(t, srv, "/settings/mcp/tokens", url.Values{"name": {""}, "scope": {"read"}})
	if code != 422 || !strings.Contains(body, "name is required") {
		t.Errorf("blank token name: %d", code)
	}

	// sidebar preference is saved server-side and rendered on the next page
	if code, _ := postForm(t, srv, "/settings", url.Values{"key": {"sidebar"}, "value": {"collapsed"}}); code != 204 {
		t.Fatalf("save pref: %d", code)
	}
	_, body = page(t, srv, "/")
	if !strings.Contains(body, `data-sb="collapsed"`) {
		t.Error("collapsed sidebar not rendered from settings")
	}
	if code, _ := postForm(t, srv, "/settings", url.Values{"key": {"nope"}, "value": {"x"}}); code != 400 {
		t.Errorf("unknown pref should be 400, got %d", code)
	}
}

func TestFaviconSettingsAndRoute(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	srv, err := web.New(d, auth.None{}, web.WithDataDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := postForm(t, srv, "/settings/favicons", url.Values{"favicon_fetch": {"on"}, "favicon_private": {"on"}}); code != 303 {
		t.Fatalf("save favicons: %d", code)
	}
	code, body := page(t, srv, "/settings/favicons")
	if code != 200 || !strings.Contains(body, `value="on" selected`) {
		t.Errorf("settings should reflect saved values: %d", code)
	}
	if code, _ := postForm(t, srv, "/settings/favicons", url.Values{"favicon_fetch": {"maybe"}, "favicon_private": {"off"}}); code != 400 {
		t.Errorf("bad value should be 400, got %d", code)
	}

	if err := os.MkdirAll(filepath.Join(dir, "icons"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "icons", "abc.png"), []byte{0x89, 'P', 'N', 'G'}, 0o644); err != nil {
		t.Fatal(err)
	}
	code, _ = page(t, srv, "/icons/abc.png")
	if code != 200 {
		t.Errorf("icon route: %d", code)
	}
}

func TestFaviconSettingsCountsAndRefetch(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	ctx := context.Background()
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "acme"})
	a, _ := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http", Domain: "acme.example.com"})
	if _, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "db", Port: 5432}); err != nil {
		t.Fatal(err)
	}
	if err := inv.SetServiceIcon(ctx, "test", a.ID, "/icons/acme.png"); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{}, web.WithDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	code, body := page(t, srv, "/settings/favicons")
	if code != 200 || !strings.Contains(body, "1 of 2 services") {
		t.Fatalf("settings counts: %d", code)
	}
	if code, _ := postForm(t, srv, "/settings/favicons/refetch", nil); code != 303 {
		t.Errorf("refetch: %d", code)
	}
}

func TestServicesPage(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "acme", Addresses: []inventory.Address{{Address: "10.0.0.5"}}})
	if _, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http", Domain: "acme.example.com", Tags: []string{"rails"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "postgres", Port: 5432, Protocol: "tcp"}); err != nil {
		t.Fatal(err)
	}
	code, body := page(t, srv, "/services")
	if code != 200 || !strings.Contains(body, "web") || !strings.Contains(body, "postgres") || !strings.Contains(body, "acme.example.com") || !strings.Contains(body, "/hosts/1") {
		t.Fatalf("services page: %d", code)
	}
	code, body = page(t, srv, "/services?q=rails")
	if code != 200 || strings.Contains(body, "postgres") || !strings.Contains(body, "web") {
		t.Errorf("filter: %d", code)
	}
}
