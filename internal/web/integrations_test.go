package web_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/web"
)

func TestIntegrationsPageAndAPI(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	m := integrations.NewManager(d, integrations.NewMemorySecrets())
	m.Register(&integrations.Plugin{
		ID: "fake", Name: "Fake plugin", Description: "test",
		Fields: []integrations.Field{
			{Key: "url", Label: "URL", Kind: "url", Required: true},
			{Key: "token", Label: "Token", Kind: "password", Secret: true},
			{Key: "auto", Label: "Auto", Kind: "bool"},
		},
		Test: func(ctx context.Context, c integrations.Config) error {
			if c["token"] == "" {
				return errors.New("token missing")
			}
			return nil
		},
		Sync: func(ctx context.Context, c integrations.Config) ([]integrations.Op, error) {
			return []integrations.Op{{Action: "link", Entity: "domain", Ref: "app.example.com", Data: map[string]string{"upstream": "10.0.0.5:3000"}}}, nil
		},
	})
	srv, err := web.New(d, auth.None{}, web.WithIntegrations(m))
	if err != nil {
		t.Fatal(err)
	}

	code, body := page(t, srv, "/settings/integrations")
	if code != 200 || !strings.Contains(body, "Fake plugin") || !strings.Contains(body, "Token") {
		t.Fatalf("page: %d", code)
	}
	if code, _ := postForm(t, srv, "/settings/integrations/fake", url.Values{
		"url": {"http://caddy:2019"}, "token": {"secret-token"}, "auto": {"1"},
	}); code != 303 {
		t.Fatalf("save: %d", code)
	}
	_, body = page(t, srv, "/settings/integrations")
	if !strings.Contains(body, `value="http://caddy:2019"`) || !strings.Contains(body, "saved — leave blank to keep") {
		t.Error("saved config/secrets not shown correctly")
	}
	if code, _ := postForm(t, srv, "/settings/integrations/fake/test", nil); code != 303 {
		t.Errorf("test: %d", code)
	}
	if code, _ := postForm(t, srv, "/settings/integrations/fake/sync", nil); code != 303 {
		t.Errorf("sync: %d", code)
	}
	_, body = page(t, srv, "/settings/integrations")
	if !strings.Contains(body, "app.example.com") || !strings.Contains(body, "synced") {
		t.Error("sync ops not rendered")
	}
	code, list := doList(t, srv, "/api/v1/integrations")
	if code != 200 || len(list) != 1 {
		t.Fatalf("api list: %d %v", code, list)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/integrations/fake/sync", ""); code != 200 {
		t.Errorf("api sync: %d", code)
	}
}

func TestPartialSyncShowsWarning(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	m := integrations.NewManager(d, integrations.NewMemorySecrets())
	m.Register(&integrations.Plugin{
		ID: "flaky", Name: "Flaky", Fields: []integrations.Field{{Key: "url", Label: "URL", Required: true}},
		Sync: func(ctx context.Context, c integrations.Config) ([]integrations.Op, error) {
			return []integrations.Op{{Action: "create", Entity: "host", Ref: "partial-host"}}, errors.New("one zone was skipped")
		},
	})
	srv, err := web.New(d, auth.None{}, web.WithIntegrations(m))
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := postForm(t, srv, "/settings/integrations/flaky", url.Values{"url": {"http://x"}}); code != 303 {
		t.Fatal("save")
	}
	code, loc := redirect(t, srv, "/settings/integrations/flaky/sync", nil)
	if code != 303 || !strings.Contains(loc, "msg=") || !strings.Contains(loc, "one+zone+was+skipped") {
		t.Fatalf("warning not surfaced: %d %s", code, loc)
	}
	_, body := page(t, srv, "/changesets")
	if !strings.Contains(body, "partial-host") {
		t.Error("changeset op missing")
	}
	code, api := do(t, srv, "POST", "/api/v1/integrations/flaky/sync", "")
	if code != 200 || !strings.Contains(fmt.Sprint(api["warning"]), "one zone was skipped") {
		t.Errorf("api partial sync warning: %d %v", code, api)
	}
}

func TestIntegrationSuggestions(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	ctx := context.Background()
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "pve"})
	if _, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "Proxmox", Port: 8006, Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	m := integrations.NewManager(d, integrations.NewMemorySecrets())
	srv, err := web.New(d, auth.None{}, web.WithIntegrations(m))
	if err != nil {
		t.Fatal(err)
	}
	code, body := page(t, srv, "/settings/integrations")
	if code != 200 || !strings.Contains(body, "Port 8006 on pve") {
		t.Errorf("suggestion missing: %d", code)
	}
}
