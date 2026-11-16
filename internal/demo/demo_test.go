package demo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/demo"
	"github.com/dawidof/home-services/internal/inventory"
)

func TestSeedFillsAnEmptyInventoryOnce(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	inv := inventory.New(d)
	if err := demo.Seed(ctx, inv); err != nil {
		t.Fatal(err)
	}
	st, err := inv.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Hosts != 2 || st.Services != 5 || st.Projects != 1 || st.Locations != 1 || st.Domains != 1 {
		t.Fatalf("demo stats: %+v", st)
	}
	hosts, err := inv.ListHosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hosts {
		full, err := inv.GetHost(ctx, h.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range full.Addresses {
			if !strings.HasPrefix(a.Address, "192.0.2.") {
				t.Errorf("demo address %q is outside TEST-NET-1", a.Address)
			}
		}
	}
	if err := demo.Seed(ctx, inv); err == nil || !strings.Contains(err.Error(), "empty data directory") {
		t.Fatalf("second seed should refuse, got %v", err)
	}
}
