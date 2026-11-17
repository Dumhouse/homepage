package inventory_test

import (
	"context"
	"testing"

	"github.com/Dumhouse/homepage/internal/inventory"
)

// Addresses that already belong to a host are not "new devices", whether the host
// existed before the scan or was added afterwards.
func TestDiscoveryIgnoresKnownAddresses(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	port := []inventory.PortInfo{{Port: 80, Service: "http"}}
	if _, err := s.CreateHost(ctx, "test", inventory.Host{Name: "nas", Addresses: []inventory.Address{{Address: "192.168.1.20"}}}); err != nil {
		t.Fatal(err)
	}
	if n, _, err := s.UpsertDiscovery(ctx, "192.168.1.20", "", port); err != nil || n != 0 {
		t.Fatalf("known address counted as new: n=%d err=%v", n, err)
	}
	if n, _, err := s.UpsertDiscovery(ctx, "192.168.1.99", "", port); err != nil || n != 1 {
		t.Fatalf("unknown address: n=%d err=%v", n, err)
	}
	// documented after it was already in the inbox
	if _, _, err := s.UpsertDiscovery(ctx, "192.168.1.50", "", port); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateHost(ctx, "test", inventory.Host{Name: "printer", Addresses: []inventory.Address{{Address: "192.168.1.50"}}}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListDiscovered(ctx, "new")
	if err != nil || len(list) != 1 || list[0].Address != "192.168.1.99" {
		t.Fatalf("inbox should hold only the unknown device: %+v %v", list, err)
	}
	if reg, _ := s.ListDiscovered(ctx, "registered"); len(reg) != 2 || reg[0].HostID == nil {
		t.Fatalf("known devices should be linked to their host: %+v", reg)
	}
}
