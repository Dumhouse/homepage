package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Dumhouse/homepage/internal/inventory"
)

func TestDomains(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	expires := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	d, err := s.CreateDomain(ctx, "test", inventory.Domain{
		Name: "https://Example.COM/", Registrar: "OVH", Account: "billing@example.com",
		ExpiresAt: expires, AutoRenew: true, PriceCents: 1299, Currency: "eur",
		PaymentLabel: "Revolut ··42", Nameservers: "ns1.example.com", TransferLock: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "example.com" || d.Currency != "EUR" || !d.AutoRenew || !d.TransferLock {
		t.Fatalf("normalize: %+v", d)
	}
	if d.DaysLeft < 8 || d.DaysLeft > 10 {
		t.Errorf("days left: %d", d.DaysLeft)
	}
	if _, err := s.CreateDomain(ctx, "test", inventory.Domain{Name: "example.com"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("duplicate domain: %v", err)
	}

	h, _ := s.CreateHost(ctx, "test", inventory.Host{Name: "acme"})
	rec, err := s.AddDomainRecord(ctx, "test", d.ID, inventory.DomainRecord{
		Name: "www", Type: "a", Content: "10.0.0.5", MatchedHost: &h.ID,
	})
	if err != nil || rec.Type != "A" || rec.MatchedHost == nil {
		t.Fatalf("record: %v %+v", err, rec)
	}
	if _, err := s.AddDomainRecord(ctx, "test", d.ID, inventory.DomainRecord{Name: "x", Type: "WAT"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("bad type: %v", err)
	}
	if _, err := s.AddDomainRecord(ctx, "test", d.ID, inventory.DomainRecord{Name: "", Type: "A"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("empty name: %v", err)
	}
	missing := int64(999)
	if _, err := s.AddDomainRecord(ctx, "test", d.ID, inventory.DomainRecord{Name: "x", Type: "A", MatchedHost: &missing}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("missing host: %v", err)
	}

	got, err := s.GetDomain(ctx, d.ID)
	if err != nil || len(got.Records) != 1 || got.Records[0].Name != "www" {
		t.Fatalf("get with records: %v %+v", err, got)
	}
	if exp, _ := s.ExpiringDomains(ctx, 30); len(exp) != 1 || exp[0].ID != d.ID {
		t.Errorf("expiring: %+v", exp)
	}
	if exp, _ := s.ExpiringDomains(ctx, 5); len(exp) != 0 {
		t.Errorf("outside window: %+v", exp)
	}

	got.Registrar = "Cloudflare"
	got.Source = "cloudflare"
	if up, err := s.UpdateDomain(ctx, "test", d.ID, *got); err != nil || up.Registrar != "Cloudflare" {
		t.Fatalf("update: %v %+v", err, up)
	}
	if err := s.DeleteDomainRecord(ctx, "test", rec.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetDomain(ctx, d.ID); len(got.Records) != 0 {
		t.Errorf("record should be soft deleted: %+v", got.Records)
	}

	if err := s.DeleteDomain(ctx, "test", d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDomain(ctx, d.ID); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("deleted domain: %v", err)
	}
	if list, _ := s.ListDomains(ctx); len(list) != 0 {
		t.Errorf("list after delete: %+v", list)
	}
	if err := s.RestoreDomain(ctx, "test", d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDomain(ctx, d.ID); err != nil {
		t.Errorf("restore: %v", err)
	}
}

func TestDomainValidation(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	bad := []inventory.Domain{
		{},
		{Name: "not a domain"},
		{Name: "example"},
		{Name: "example.com", Currency: "EURO"},
		{Name: "example.com", PriceCents: -1},
		{Name: "example.com", ExpiresAt: "16-11-2026"},
		{Name: "example.com", Source: "cloudflare-import"},
	}
	for i, d := range bad {
		if _, err := s.CreateDomain(ctx, "test", d); !errors.Is(err, inventory.ErrInvalid) {
			t.Errorf("case %d (%+v): want invalid, got %v", i, d, err)
		}
	}
}

func TestServiceIconStats(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	h, _ := s.CreateHost(ctx, "test", inventory.Host{Name: "acme"})
	a, _ := s.AddService(ctx, "test", h.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http", Domain: "acme.example.com"})
	if _, err := s.AddService(ctx, "test", h.ID, inventory.Service{Name: "db", Port: 5432}); err != nil {
		t.Fatal(err)
	}
	if total, withIcon, err := s.ServiceIconStats(ctx); err != nil || total != 2 || withIcon != 0 {
		t.Fatalf("stats: %v %d/%d", err, withIcon, total)
	}
	if err := s.SetServiceIcon(ctx, "test", a.ID, "/icons/acme.png"); err != nil {
		t.Fatal(err)
	}
	if total, withIcon, _ := s.ServiceIconStats(ctx); total != 2 || withIcon != 1 {
		t.Errorf("after icon: %d/%d", withIcon, total)
	}
	list, err := s.ServicesWithDomain(ctx)
	if err != nil || len(list) != 1 || list[0].Domain != "acme.example.com" || list[0].Icon == "" {
		t.Fatalf("services with domain: %v %+v", err, list)
	}
}

func TestDomainPaidValueValidation(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	if _, err := s.CreateDomain(ctx, "test", inventory.Domain{Name: "x.example", PaidAt: "11/16/2026"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("bad paid_at: %v", err)
	}
	if _, err := s.CreateDomain(ctx, "test", inventory.Domain{Name: "x.example", ChargedCents: -1}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("negative charged: %v", err)
	}
	d, err := s.CreateDomain(ctx, "test", inventory.Domain{Name: "x.example", Currency: "usd", ChargedCents: 500, ChargedCurrency: "eur", ChargedNote: "as charged"})
	if err != nil || d.Currency != "USD" || d.ChargedCurrency != "EUR" {
		t.Fatalf("currency normalization: %v %+v", err, d)
	}
	got, _ := s.GetDomain(ctx, d.ID)
	if got.ChargedCents != 500 || got.ChargedNote != "as charged" {
		t.Errorf("paid value round-trip: %+v", got)
	}
}

func TestAllServices(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	loc, _ := s.CreateLocation(ctx, "test", inventory.Location{Name: "Lab"})
	h, _ := s.CreateHost(ctx, "test", inventory.Host{Name: "acme", LocationID: &loc.ID, Addresses: []inventory.Address{{Address: "10.0.0.5"}}})
	if _, err := s.AddService(ctx, "test", h.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http", Domain: "acme.example.com", Tags: []string{"rails"}}); err != nil {
		t.Fatal(err)
	}
	gone, _ := s.AddService(ctx, "test", h.ID, inventory.Service{Name: "old", Port: 9999})
	if err := s.DeleteService(ctx, "test", gone.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.AllServices(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("all services: %v %+v", err, list)
	}
	v := list[0]
	if v.HostName != "acme" || v.Address != "10.0.0.5" || v.Location != "Lab" || len(v.Tags) != 1 || v.Tags[0] != "rails" {
		t.Errorf("joined service: %+v", v)
	}
}
