package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Dumhouse/homepage/internal/inventory"
)

func TestCosts(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	p, _ := s.CreateProject(ctx, "test", inventory.Project{Name: "Acme"})
	m, err := s.CreateCost(ctx, "test", inventory.Cost{
		Label: "Hetzner", AmountCents: 451, Currency: "eur", Cycle: "monthly", ProjectID: &p.ID,
		AutoRenew: true, PaymentLabel: "Revolut",
	})
	if err != nil || m.Currency != "EUR" || m.ProjectID == nil {
		t.Fatalf("create: %v %+v", err, m)
	}
	y, _ := s.CreateCost(ctx, "test", inventory.Cost{Label: "Domain", AmountCents: 1200, Cycle: "yearly", NextRenewal: "2027-01-01"})
	o, _ := s.CreateCost(ctx, "test", inventory.Cost{Label: "SSD", AmountCents: 9000, Cycle: "one_time"})
	if y.DaysLeft < 0 || o.Cycle != "one_time" {
		t.Fatalf("costs: %+v %+v", y, o)
	}
	if list, _ := s.ListCosts(ctx); len(list) != 3 {
		t.Errorf("list: %+v", list)
	}
	y.AmountCents = 1500
	if up, err := s.UpdateCost(ctx, "test", y.ID, *y); err != nil || up.AmountCents != 1500 {
		t.Errorf("update: %v %+v", err, up)
	}
	bad := []inventory.Cost{
		{},
		{Label: "x", Cycle: "weekly"},
		{Label: "x", Currency: "EURO"},
		{Label: "x", AmountCents: -1},
		{Label: "x", NextRenewal: "01/01/2027"},
	}
	for i, c := range bad {
		if _, err := s.CreateCost(ctx, "test", c); !errors.Is(err, inventory.ErrInvalid) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	missing := int64(999)
	if _, err := s.CreateCost(ctx, "test", inventory.Cost{Label: "x", ProjectID: &missing}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("missing project: %v", err)
	}
	if err := s.DeleteCost(ctx, "test", o.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCost(ctx, o.ID); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("deleted: %v", err)
	}
	if err := s.RestoreCost(ctx, "test", o.ID); err != nil {
		t.Fatal(err)
	}
}
