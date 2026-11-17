package suggest_test

import (
	"context"
	"testing"

	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/suggest"
)

func TestSuggestions(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inv := inventory.New(d)
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "pve", Addresses: []inventory.Address{{Address: "10.0.0.250"}}})
	if _, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "Proxmox", Port: 8006, Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.CreateDomain(ctx, "test", inventory.Domain{Name: "example.com"}); err != nil {
		t.Fatal(err)
	}
	list, err := suggest.For(ctx, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	plugins := map[string]bool{}
	for _, s := range list {
		plugins[s.Plugin] = true
	}
	if !plugins["proxmox"] || !plugins["cloudflare"] || !plugins["power"] {
		t.Fatalf("suggestions: %+v", list)
	}
	list, _ = suggest.For(ctx, inv, map[string]bool{"proxmox": true})
	for _, s := range list {
		if s.Plugin == "proxmox" {
			t.Error("configured plugin should not be suggested")
		}
	}
}
