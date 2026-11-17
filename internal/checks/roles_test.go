package checks_test

import (
	"context"
	"testing"

	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/inventory"
)

// A service with both a LAN port and a domain gets two auto checks, so the
// dashboard can tell "LAN down" from "public route down". Removing the domain
// drops the public one again.
func TestReconcileLanAndPublicRoles(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	inv := inventory.New(d)
	cstore := checks.New(d)
	h, err := inv.CreateHost(ctx, "test", inventory.Host{Name: "vault", Addresses: []inventory.Address{{Address: "10.0.0.7"}}})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "vaultwarden", Port: 8080, Protocol: "http", Domain: "vault.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cstore.Reconcile(ctx, inv); err != nil {
		t.Fatal(err)
	}
	list, _ := cstore.List(ctx, "service", svc.ID)
	roles := map[string]string{}
	for _, c := range list {
		roles[c.Role()], _ = c.Params["url"].(string)
	}
	if roles["lan"] != "http://10.0.0.7:8080" || roles["public"] != "http://vault.example.com" || len(list) != 2 {
		t.Fatalf("want a LAN and a public check, got %+v", roles)
	}
	// reconcile again: nothing changes
	if n, _ := cstore.Reconcile(ctx, inv); n != 0 {
		t.Fatalf("second reconcile changed %d checks", n)
	}
	svc.Domain = ""
	if _, err := inv.UpdateService(ctx, "test", svc.ID, *svc); err != nil {
		t.Fatal(err)
	}
	if _, err := cstore.Reconcile(ctx, inv); err != nil {
		t.Fatal(err)
	}
	list, _ = cstore.List(ctx, "service", svc.ID)
	if len(list) != 1 || list[0].Role() != "lan" {
		t.Fatalf("public check should be gone: %+v", list)
	}
}
