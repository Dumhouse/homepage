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

// Address links, costs and facts from the cloud, VPN and metrics plugins.
func TestAddressCostAndFacts(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inv := inventory.New(d)
	s := changesets.New(d, inv)
	nas, _ := inv.CreateHost(ctx, "ui", inventory.Host{Name: "NAS", Kind: "bare", Addresses: []inventory.Address{{Address: "192.168.1.20"}}})

	// facts for a known host are written at once; an unknown one waits
	n, missing, rest := s.ApplyFacts(ctx, "plugin:prometheus", []changesets.Op{
		{Entity: "facts", Ref: "nas", Data: map[string]string{"host": "nas", "os": "Debian 12", "disk_used_pct": "42", "uptime": "3 days"}},
		{Entity: "facts", Ref: "ghost", Data: map[string]string{"host": "ghost", "os": "Alpine"}},
		{Entity: "host", Ref: "ghost", Data: map[string]string{"kind": "bare"}},
	})
	if n != 1 || len(missing) != 1 || len(rest) != 2 {
		t.Fatalf("facts: %d %v %d", n, missing, len(rest))
	}
	if f, _ := inv.GetHostFacts(ctx, nas.ID); f == nil || f.OS != "Debian 12" || f.DiskUsedPct != 42 {
		t.Fatalf("facts not saved: %+v", f)
	}

	ops := []changesets.Op{
		{Action: "link", Entity: "address", Ref: "nas", Data: map[string]string{"host": "nas.tail1.ts.net", "address": "100.64.0.5", "kind": "tailscale", "optional": "1"}},
		{Action: "link", Entity: "address", Ref: "pixel", Data: map[string]string{"host": "pixel", "address": "100.64.0.9", "kind": "tailscale", "optional": "1"}},
		{Action: "create", Entity: "host", Ref: "web-1", Data: map[string]string{"kind": "vps", "address": "203.0.113.5", "address_kind": "public", "address2": "10.0.0.2"}},
		{Action: "create", Entity: "cost", Ref: "web-1", Data: map[string]string{"label": "web-1", "host": "web-1", "amount_cents": "451", "currency": "EUR", "source": "plugin:hetzner"}},
	}
	fresh := s.Fresh(ctx, ops)
	if len(fresh) != 3 { // pixel isn't documented and is optional
		t.Fatalf("fresh: %+v", fresh)
	}
	cs, _ := s.Create(ctx, "ui", "sync", "plugin:x", fresh)
	applied, err := s.Apply(ctx, "ui", cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range applied.Ops {
		if op.Error != "" {
			t.Fatalf("%s %s: %s", op.Entity, op.Ref, op.Error)
		}
	}
	h, _ := inv.GetHost(ctx, nas.ID)
	if len(h.Addresses) != 2 || h.Addresses[1].Kind != "tailscale" {
		t.Fatalf("tailscale address: %+v", h.Addresses)
	}
	costs, _ := inv.ListCosts(ctx)
	if len(costs) != 1 || costs[0].TargetEntity != "host" || costs[0].TargetID == 0 || costs[0].AmountCents != 451 {
		t.Fatalf("cost: %+v", costs)
	}
	if again := s.Fresh(ctx, ops); len(again) != 0 {
		t.Fatalf("second sync should be empty: %+v", again)
	}
}

// A DHCP lease fills an empty LAN address but never replaces one you entered;
// an empty error from a later sync clears the host warning.
func TestLeaseOnlyIfEmptyAndWarningClears(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inv := inventory.New(d)
	s := changesets.New(d, inv)
	nas, _ := inv.CreateHost(ctx, "ui", inventory.Host{Name: "nas", Kind: "bare", Addresses: []inventory.Address{{Kind: "lan", Address: "192.168.1.20"}}})
	pve, _ := inv.CreateHost(ctx, "ui", inventory.Host{Name: "pve", Kind: "bare"})
	lease := func(host, ip string) changesets.Op {
		return changesets.Op{Action: "link", Entity: "address", Ref: host,
			Data: map[string]string{"host": host, "address": ip, "kind": "lan", "optional": "1", "only_if_empty": "1"}}
	}
	fresh := s.Fresh(ctx, []changesets.Op{lease("nas", "192.168.1.77"), lease("pve.lan", "192.168.1.30")})
	if len(fresh) != 1 || fresh[0].Ref != "pve.lan" {
		t.Fatalf("fresh: %+v", fresh)
	}
	cs, _ := s.Create(ctx, "ui", "dhcp", "plugin:x", fresh)
	s.Apply(ctx, "ui", cs.ID)
	if h, _ := inv.GetHost(ctx, pve.ID); len(h.Addresses) != 1 || h.Addresses[0].Address != "192.168.1.30" {
		t.Fatalf("pve: %+v", h.Addresses)
	}
	if h, _ := inv.GetHost(ctx, nas.ID); len(h.Addresses) != 1 || h.Addresses[0].Address != "192.168.1.20" {
		t.Fatalf("nas address must not change: %+v", h.Addresses)
	}
	s.ApplyFacts(ctx, "x", []changesets.Op{{Entity: "facts", Ref: "nas", Data: map[string]string{"host": "nas", "error": "disk sda fails SMART"}}})
	s.ApplyFacts(ctx, "x", []changesets.Op{{Entity: "facts", Ref: "nas", Data: map[string]string{"host": "nas", "os": "Debian"}}})
	if f, _ := inv.GetHostFacts(ctx, nas.ID); f.Error == "" {
		t.Fatal("a source that says nothing about errors must keep the warning")
	}
	s.ApplyFacts(ctx, "x", []changesets.Op{{Entity: "facts", Ref: "nas", Data: map[string]string{"host": "nas", "error": ""}}})
	if f, _ := inv.GetHostFacts(ctx, nas.ID); f.Error != "" || f.OS != "Debian" {
		t.Fatalf("cleared: %+v", f)
	}
}
