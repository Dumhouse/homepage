package inventory_test

import (
	"context"
	"testing"

	"github.com/Dumhouse/homepage/internal/inventory"
)

func TestSearch(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	h, err := s.CreateHost(ctx, "test", inventory.Host{
		Name: "acme-prod", Hostname: "acme.example.com",
		Addresses: []inventory.Address{{Address: "192.168.1.240"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddService(ctx, "test", h.ID, inventory.Service{Name: "sidekiq", Port: 3001}); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "test", inventory.Project{Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProjectEnv(ctx, "test", p.ID, inventory.ProjectEnv{Name: "production", BaseURL: "https://acme.example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAppLink(ctx, "test", p.ID, inventory.AppLink{Label: "Sidekiq", Path: "/sidekiq", Kind: "jobs"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLocation(ctx, "test", inventory.Location{Name: "Proxmox .250", Kind: "proxmox"}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		q    string
		want string
	}{
		{"acme", "project"},
		{"acme-prod", "host"},
		{"192.168.1.240", "host"},
		{"acme.example.com", "host"},
		{"sidekiq", "service"},
		{"proxmox", "location"},
	}
	for _, c := range cases {
		got, err := s.Search(ctx, c.q, 20)
		if err != nil {
			t.Fatalf("%q: %v", c.q, err)
		}
		found := false
		for _, r := range got {
			if r.Kind == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q should find a %s: %+v", c.q, c.want, got)
		}
	}

	// exact project name ranks first
	got, _ := s.Search(ctx, "acme", 20)
	if len(got) == 0 || got[0].Kind != "project" || got[0].Title != "Acme" {
		t.Errorf("exact name should rank first: %+v", got)
	}
	// two words match one app link, and its URL is resolved against the env
	got, _ = s.Search(ctx, "acme sidekiq", 20)
	if len(got) == 0 {
		t.Fatal("no hits for two words")
	}
	var link *inventory.SearchResult
	for i := range got {
		if got[i].Kind == "app_link" {
			link = &got[i]
		}
	}
	if link == nil || link.URL != "https://acme.example.com/sidekiq" {
		t.Errorf("app link URL: %+v", got)
	}
	if got, _ := s.Search(ctx, "sidekiq", 1); len(got) != 1 {
		t.Errorf("limit should apply: %+v", got)
	}
	if got, _ := s.Search(ctx, "   ", 10); len(got) != 0 {
		t.Errorf("blank query: %+v", got)
	}
	if got, _ := s.Search(ctx, "nothing-matches-this", 10); len(got) != 0 {
		t.Errorf("no hits expected: %+v", got)
	}
}

func TestConflictWarningsAndFreePort(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	a, err := s.CreateHost(ctx, "test", inventory.Host{Name: "a", Addresses: []inventory.Address{{Address: "10.0.0.5"}}})
	if err != nil || len(a.Warnings) != 0 {
		t.Fatalf("first host: %v %+v", err, a.Warnings)
	}
	if _, err := s.AddService(ctx, "test", a.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http"}); err != nil {
		t.Fatal(err)
	}

	b, err := s.CreateHost(ctx, "test", inventory.Host{
		Name: "b", Addresses: []inventory.Address{{Address: "10.0.0.5"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Warnings) != 1 || b.Warnings[0] != "10.0.0.5 is also used by a" {
		t.Errorf("duplicate address warning: %+v", b.Warnings)
	}
	if _, err := s.AddService(ctx, "test", b.ID, inventory.Service{Name: "web2", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	b, err = s.UpdateHost(ctx, "test", b.ID, *b)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Warnings) != 2 {
		t.Errorf("duplicate port should warn too: %+v", b.Warnings)
	}

	// free port skips ports used on the same address, on any host
	p, err := s.FreePort(ctx, b.ID, 3000)
	if err != nil || p != 3001 {
		t.Fatalf("free port: %v %d", err, p)
	}
	if _, err := s.AddService(ctx, "test", a.ID, inventory.Service{Name: "extra", Port: 3001}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.FreePort(ctx, b.ID, 3000); p != 3002 {
		t.Errorf("free port should skip 3001: %d", p)
	}
	if _, err := s.FreePort(ctx, 999, 3000); err == nil {
		t.Error("unknown host should be an error")
	}

	// moving the host clears the address warning
	b.Addresses[0].Address = "10.0.0.6"
	b, err = s.UpdateHost(ctx, "test", b.ID, *b)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Warnings) != 0 {
		t.Errorf("warning should clear after the move: %+v", b.Warnings)
	}
}

func TestAuditAPIStore(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	h, err := s.CreateHost(ctx, "mcp:claude", inventory.Host{Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	h.Description = "changed"
	if _, err := s.UpdateHost(ctx, "mcp:claude", h.ID, *h); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHost(ctx, "ui", h.ID); err != nil {
		t.Fatal(err)
	}
	entries, err := s.ListAudit(ctx, "host", h.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %+v", entries)
	}
	if entries[0].Action != "delete" || entries[0].Actor != "ui" {
		t.Errorf("newest first: %+v", entries[0])
	}
	if entries[2].Action != "create" || entries[2].Actor != "mcp:claude" || len(entries[2].Diff) == 0 {
		t.Errorf("oldest entry: %+v", entries[2])
	}
	if all, _ := s.ListAudit(ctx, "", 0, 1); len(all) != 1 {
		t.Errorf("limit: %+v", all)
	}
}
