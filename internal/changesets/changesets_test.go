package changesets_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dawidof/home-services/internal/changesets"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/inventory"
)

func TestApplyUndo(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inv := inventory.New(d)
	s := changesets.New(d, inv)

	cs, err := s.Create(ctx, "mcp:test", "Proxmox import", "proxmox", []changesets.Op{
		{Action: "create", Entity: "host", Ref: "acme", Data: map[string]string{"kind": "lxc", "location": "pve", "address": "10.0.0.20"}},
		{Action: "create", Entity: "domain", Ref: "example.com", Data: map[string]string{"source": "cloudflare"}},
		{Action: "create", Entity: "dns_record", Ref: "www", Data: map[string]string{"domain": "example.com", "type": "A", "content": "10.0.0.20"}},
		{Action: "create", Entity: "service", Ref: "web", Data: map[string]string{"host": "acme", "port": "3000", "protocol": "http"}},
		{Action: "frobnicate", Entity: "mystery"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cs.Status != "pending" || len(cs.Ops) != 5 {
		t.Fatalf("create: %+v", cs)
	}
	applied, err := s.Apply(ctx, "ui", cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != "applied" || applied.AppliedAt == "" {
		t.Errorf("apply status: %+v", applied)
	}
	if applied.Ops[4].Error == "" {
		t.Error("unsupported op should record an error")
	}
	hosts, _ := inv.ListHosts(ctx)
	if len(hosts) != 1 || hosts[0].Name != "acme" || hosts[0].Location == nil {
		t.Fatalf("host not created: %+v", hosts)
	}
	fullHost, _ := inv.GetHost(ctx, hosts[0].ID)
	if len(fullHost.Services) != 1 || fullHost.Services[0].Port != 3000 {
		t.Errorf("service not created: %+v", fullHost.Services)
	}
	domains, _ := inv.ListDomains(ctx)
	if len(domains) != 1 || domains[0].Name != "example.com" {
		t.Fatalf("domain not created: %+v", domains)
	}
	dom, _ := inv.GetDomain(ctx, domains[0].ID)
	if len(dom.Records) != 1 || dom.Records[0].Content != "10.0.0.20" {
		t.Errorf("record not created: %+v", dom.Records)
	}

	if err := s.Undo(ctx, "ui", cs.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := inv.ListHosts(ctx); len(list) != 0 {
		t.Errorf("undo should soft delete host: %+v", list)
	}
	if list, _ := inv.ListDomains(ctx); len(list) != 0 {
		t.Errorf("undo should soft delete domain: %+v", list)
	}
	got, _ := s.Get(ctx, cs.ID)
	if got.Status != "undone" {
		t.Errorf("status after undo: %s", got.Status)
	}
	if err := s.Undo(ctx, "ui", cs.ID); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("second undo: %v", err)
	}
}

func TestReject(t *testing.T) {
	ctx := context.Background()
	d, _ := db.Open(t.TempDir() + "/t.db")
	defer d.Close()
	inv := inventory.New(d)
	s := changesets.New(d, inv)
	cs, err := s.Create(ctx, "mcp:reader", "nope", "mcp", []changesets.Op{{Action: "create", Entity: "host", Ref: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reject(ctx, "ui", cs.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, cs.ID)
	if got.Status != "rejected" {
		t.Errorf("reject: %s", got.Status)
	}
	if _, err := s.Apply(ctx, "ui", cs.ID); err == nil {
		t.Error("rejected set should not apply")
	}
	if list, _ := s.List(ctx, "pending"); len(list) != 0 {
		t.Errorf("pending list: %d", len(list))
	}
	if _, err := s.Create(ctx, "ui", "empty", "mcp", nil); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("empty set: %v", err)
	}
}

// A second sync proposes nothing new, and undoing an import never deletes a
// host that was already documented before it.
func TestSyncIsIdempotentAndUndoKeepsExisting(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inv := inventory.New(d)
	s := changesets.New(d, inv)
	old, err := inv.CreateHost(ctx, "ui", inventory.Host{Name: "pve", Kind: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	ops := []changesets.Op{
		{Action: "create", Entity: "host", Ref: "pve", Data: map[string]string{"kind": "bare", "address": "192.168.1.250", "location": "pve"}},
		{Action: "create", Entity: "host", Ref: "acme", Data: map[string]string{"kind": "lxc", "address": "192.168.1.240", "location": "pve", "proxmox_vmid": "101"}},
	}
	fresh := s.Fresh(ctx, ops)
	if len(fresh) != 2 { // pve exists but has no address yet: still worth applying
		t.Fatalf("fresh: %+v", fresh)
	}
	cs, _ := s.Create(ctx, "ui", "sync", "plugin:proxmox", fresh)
	if _, err := s.Apply(ctx, "ui", cs.ID); err != nil {
		t.Fatal(err)
	}
	if h, _ := inv.GetHost(ctx, old.ID); len(h.Addresses) != 1 || h.Addresses[0].Address != "192.168.1.250" {
		t.Fatalf("existing host should get the address: %+v", h.Addresses)
	}
	if again := s.Fresh(ctx, ops); len(again) != 0 {
		t.Fatalf("second sync should find nothing new: %+v", again)
	}
	if err := s.Undo(ctx, "ui", cs.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.GetHost(ctx, old.ID); err != nil {
		t.Fatalf("undo deleted a pre-existing host: %v", err)
	}
	hosts, _ := inv.ListHosts(ctx)
	if len(hosts) != 1 {
		t.Fatalf("undo should remove only acme, left %d hosts", len(hosts))
	}
}

func TestConnectionOp(t *testing.T) {
	ctx := context.Background()
	d, _ := db.Open(t.TempDir() + "/t.db")
	defer d.Close()
	inv := inventory.New(d)
	s := changesets.New(d, inv)
	proj, _ := inv.CreateProject(ctx, "test", inventory.Project{Name: "Acme"})
	cs, err := s.Create(ctx, "mcp:test", "Connection request", "mcp", []changesets.Op{{
		Action: "create", Entity: "connection", Ref: "acme",
		Data: map[string]string{
			"project": "acme", "target_kind": "url", "url": "https://git.example.com",
			"role": "code", "detail": "Forgejo", "impact_note": "no deploys",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, "ui", cs.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := inv.GetProject(ctx, proj.ID)
	if len(got.Connections) != 1 || got.Connections[0].Role != "code" || got.Connections[0].URL != "https://git.example.com" {
		t.Fatalf("connection not created: %+v", got.Connections)
	}
	if err := s.Undo(ctx, "ui", cs.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := inv.GetProject(ctx, proj.ID); len(got.Connections) != 0 {
		t.Errorf("undo should remove the connection: %+v", got.Connections)
	}
}

func TestServiceOpMatchesHostByAddress(t *testing.T) {
	ctx := context.Background()
	d, _ := db.Open(t.TempDir() + "/t.db")
	defer d.Close()
	inv := inventory.New(d)
	s := changesets.New(d, inv)
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "docker-1", Addresses: []inventory.Address{{Address: "10.0.0.5"}}})
	cs, err := s.Create(ctx, "mcp:test", "Docker service", "mcp", []changesets.Op{{
		Action: "create", Entity: "service", Ref: "10.0.0.5:8080",
		Data: map[string]string{"host": "10.0.0.5", "name": "web", "port": "8080", "protocol": "http"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, "ui", cs.ID); err != nil {
		t.Fatal(err)
	}
	full, _ := inv.GetHost(ctx, h.ID)
	if len(full.Services) != 1 || full.Services[0].Port != 8080 || full.Services[0].Name != "web" {
		t.Fatalf("service not created: %+v", full.Services)
	}
	if err := s.Undo(ctx, "ui", cs.ID); err != nil {
		t.Fatal(err)
	}
	if full, _ := inv.GetHost(ctx, h.ID); len(full.Services) != 0 {
		t.Errorf("undo should remove the service: %+v", full.Services)
	}
}
