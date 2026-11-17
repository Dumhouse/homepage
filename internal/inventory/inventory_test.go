package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/inventory"
)

func newStore(t *testing.T) *inventory.Store {
	t.Helper()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return inventory.New(d)
}

func TestHostLifecycle(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	h, err := s.CreateHost(ctx, "test", inventory.Host{
		Name: "acme-staging", Kind: "lxc", Tags: []string{"staging"},
		Addresses: []inventory.Address{{Address: "192.168.1.240"}, {Kind: "tailscale", Address: "100.81.4.22"}},
		SSH:       []inventory.SSHAccess{{User: "root"}, {User: "deploy", Port: 2222}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !h.Addresses[0].IsPrimary || h.Addresses[0].Address != "192.168.1.240" {
		t.Errorf("first address should become primary: %+v", h.Addresses)
	}
	if h.SSH[0].Port != 22 || h.SSH[1].Port != 2222 || h.SSH[0].Status != "unknown" {
		t.Errorf("ssh defaults wrong: %+v", h.SSH)
	}

	v, err := s.AddService(ctx, "test", h.ID, inventory.Service{Name: "postgres", Port: 5432})
	if err != nil || v.Protocol != "tcp" {
		t.Fatalf("service: %v %+v", err, v)
	}
	got, _ := s.GetHost(ctx, h.ID)
	if len(got.Services) != 1 || got.Services[0].Port != 5432 {
		t.Errorf("host should include its service: %+v", got.Services)
	}

	// update replaces children
	got.Addresses = got.Addresses[:1]
	got.SSH = nil
	up, err := s.UpdateHost(ctx, "test", h.ID, *got)
	if err != nil || len(up.Addresses) != 1 || len(up.SSH) != 0 {
		t.Fatalf("update: %v %+v", err, up)
	}

	// soft delete + restore
	if err := s.DeleteHost(ctx, "test", h.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetHost(ctx, h.ID); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("deleted host should be hidden, got %v", err)
	}
	if list, _ := s.ListHosts(ctx); len(list) != 0 {
		t.Error("list should not show deleted")
	}
	if err := s.RestoreHost(ctx, "test", h.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetHost(ctx, h.ID); err != nil {
		t.Errorf("restored host missing: %v", err)
	}
	if err := s.RestoreHost(ctx, "test", h.ID); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("restoring a live host should be not found, got %v", err)
	}
}

func TestValidation(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	bad := []inventory.Host{
		{},
		{Name: "x", Kind: "toaster"},
		{Name: "x", Addresses: []inventory.Address{{Address: " "}}},
		{Name: "x", Addresses: []inventory.Address{{Address: "a", IsPrimary: true}, {Address: "b", IsPrimary: true}}},
		{Name: "x", Addresses: []inventory.Address{{Address: "a"}, {Address: "a"}}},
		{Name: "x", SSH: []inventory.SSHAccess{{User: "root", Port: 70000}}},
		{Name: "x", SSH: []inventory.SSHAccess{{}}},
	}
	for i, h := range bad {
		if _, err := s.CreateHost(ctx, "t", h); !errors.Is(err, inventory.ErrInvalid) {
			t.Errorf("case %d: want ErrInvalid, got %v", i, err)
		}
	}
	h, _ := s.CreateHost(ctx, "t", inventory.Host{Name: "ok"})
	if _, err := s.AddService(ctx, "t", h.ID, inventory.Service{Name: "x", Port: 99999}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("port range: %v", err)
	}
	if _, err := s.AddService(ctx, "t", 999, inventory.Service{Name: "x"}); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("unknown host: %v", err)
	}
}

func TestProjectsAndAudit(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	p, err := s.CreateProject(ctx, "mcp:laptop", inventory.Project{Name: "Acme App!"})
	if err != nil || p.Slug != "acme-app" || p.Status != "active" {
		t.Fatalf("%v %+v", err, p)
	}
	if _, err := s.CreateProject(ctx, "ui", inventory.Project{Name: "Other", Slug: "Acme App"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("duplicate slug should be invalid, got %v", err)
	}
	p.Description = "Rails + Next"
	if _, err := s.UpdateProject(ctx, "ui", p.ID, *p); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(ctx, "ui", p.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.ListProjects(ctx); len(l) != 0 {
		t.Error("deleted project listed")
	}
}
