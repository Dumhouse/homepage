package expiry_test

import (
	"context"
	"testing"
	"time"

	"github.com/Dumhouse/homepage/internal/alerts"
	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/expiry"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/settings"
)

type capture struct{ events []alerts.Event }

func (c *capture) Broadcast(ctx context.Context, e alerts.Event) error {
	c.events = append(c.events, e)
	return nil
}

func TestExpiryThresholds(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inv := inventory.New(d)
	set := settings.New(d)
	now := time.Now().UTC()
	mk := func(name string, days int, renew bool) *inventory.Domain {
		dom, err := inv.CreateDomain(ctx, "test", inventory.Domain{
			Name: name, ExpiresAt: now.AddDate(0, 0, days).Format("2006-01-02"), AutoRenew: renew,
		})
		if err != nil {
			t.Fatal(err)
		}
		return dom
	}
	six := mk("six.example", 6, false)
	mk("far.example", 40, true)
	c := &capture{}
	n, err := expiry.Run(ctx, inv, set, c, now)
	if err != nil || n != 1 {
		t.Fatalf("first run: %v sent %d %+v", err, n, c.events)
	}
	if c.events[0].State != "warning" || c.events[0].Entity != "domain" {
		t.Errorf("event: %+v", c.events[0])
	}
	if n, _ := expiry.Run(ctx, inv, set, c, now); n != 0 {
		t.Errorf("duplicate sent: %d", n)
	}

	// crossing into the 1-day window adds a second alert
	six.ExpiresAt = now.AddDate(0, 0, 1).Format("2006-01-02")
	if _, err := inv.UpdateDomain(ctx, "test", six.ID, *six); err != nil {
		t.Fatal(err)
	}
	if n, _ := expiry.Run(ctx, inv, set, c, now); n != 1 || len(c.events) != 2 {
		t.Errorf("1-day alert: sent %d total %d", n, len(c.events))
	}

	// renewing past 30 days clears the state, so a later expiry alerts again
	six.ExpiresAt = now.AddDate(0, 0, 60).Format("2006-01-02")
	if _, err := inv.UpdateDomain(ctx, "test", six.ID, *six); err != nil {
		t.Fatal(err)
	}
	_, _ = expiry.Run(ctx, inv, set, c, now)
	six.ExpiresAt = now.AddDate(0, 0, 3).Format("2006-01-02")
	if _, err := inv.UpdateDomain(ctx, "test", six.ID, *six); err != nil {
		t.Fatal(err)
	}
	if n, _ := expiry.Run(ctx, inv, set, c, now); n != 1 {
		t.Errorf("renewed domain should alert again, sent %d", n)
	}
}

func TestCertThresholds(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	set := settings.New(d)
	cs := checks.New(d)
	c, err := cs.Create(ctx, "test", checks.Check{
		TargetEntity: "host", TargetID: 1, Kind: "tls",
		Params: map[string]any{"address": "example.com:443"},
	})
	if err != nil {
		t.Fatal(err)
	}
	label := func(context.Context, string, int64) string { return "edge:443" }
	capt := &capture{}
	record := func(days int) {
		t.Helper()
		cur, err := cs.Get(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := cs.Record(ctx, *cur, checks.Result{OK: true, CertDays: days, CertIssuer: "Let's Encrypt", Message: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	record(6)
	if n, err := expiry.RunCerts(ctx, cs, label, set, capt, now); err != nil || n != 1 {
		t.Fatalf("first: %v %d", err, n)
	}
	if n, _ := expiry.RunCerts(ctx, cs, label, set, capt, now); n != 0 {
		t.Errorf("duplicate: %d", n)
	}
	record(1)
	if n, _ := expiry.RunCerts(ctx, cs, label, set, capt, now); n != 1 {
		t.Errorf("1-day alert: %d", n)
	}
	record(60)
	_, _ = expiry.RunCerts(ctx, cs, label, set, capt, now)
	record(3)
	if n, _ := expiry.RunCerts(ctx, cs, label, set, capt, now); n != 1 {
		t.Errorf("renewed cert should alert again: %d", n)
	}
	if len(capt.events) != 3 || capt.events[0].Entity != "certificate" {
		t.Errorf("events: %+v", capt.events)
	}
}
