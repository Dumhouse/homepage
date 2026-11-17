// Package expiry sends one alert per domain when it crosses the 30/7/1-day
// renewal thresholds, and exposes the data behind the calendar page.
package expiry

import (
	"context"
	"fmt"
	"time"

	"github.com/Dumhouse/homepage/internal/alerts"
	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/settings"
)

// Thresholds in days, smallest first.
var Thresholds = []int{0, 1, 7, 30}

type Notifier interface {
	Broadcast(context.Context, alerts.Event) error
}

// Run checks every live domain and sends at most one alert per threshold.
// It returns how many alerts were sent.
func Run(ctx context.Context, inv *inventory.Store, set *settings.Store, notifier Notifier, now time.Time) (int, error) {
	domains, err := inv.ListDomains(ctx)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, d := range domains {
		if d.ExpiresAt == "" {
			continue
		}
		days := d.DaysLeft
		if days > 30 {
			// renewed or extended: clear old thresholds so they can fire again
			for _, t := range Thresholds {
				_ = set.Delete(ctx, key(d.ID, t))
			}
			continue
		}
		threshold := 0
		for _, t := range Thresholds {
			if days <= t {
				threshold = t
				break
			}
		}
		if days < 0 {
			threshold = 0
		}
		k := key(d.ID, threshold)
		if v, _ := set.Get(ctx, k, ""); v != "" {
			continue
		}
		msg := fmt.Sprintf("%s expires in %d days (%s)", d.Name, days, d.ExpiresAt)
		if days < 0 {
			msg = fmt.Sprintf("%s expired %d days ago (%s)", d.Name, -days, d.ExpiresAt)
		}
		if !d.AutoRenew {
			msg += " — auto-renew is OFF"
		}
		ev := alerts.Event{
			Entity: "domain", TargetID: d.ID, Label: d.Name, Kind: "expiry",
			State: "warning", Message: msg, At: now,
		}
		if err := notifier.Broadcast(ctx, ev); err != nil {
			return sent, err
		}
		_ = set.Set(ctx, "system:expiry", k, now.UTC().Format("2006-01-02"))
		sent++
	}
	return sent, nil
}

func key(id int64, threshold int) string {
	return fmt.Sprintf("expiry:%d:%d", id, threshold)
}

// RunCerts sends one alert per TLS check when its certificate crosses
// 30/7/1/0 days, using the same settings keys as domains.
func RunCerts(ctx context.Context, chk *checks.Store, label func(context.Context, string, int64) string, set *settings.Store, notifier Notifier, now time.Time) (int, error) {
	list, err := chk.List(ctx, "", 0)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, c := range list {
		if c.Kind != "tls" || c.CertDays == -1 {
			continue
		}
		if c.CertDays > 30 {
			for _, t := range Thresholds {
				_ = set.Delete(ctx, certKey(c.ID, t))
			}
			continue
		}
		days := c.CertDays
		threshold := 0
		for _, t := range Thresholds {
			if days <= t {
				threshold = t
				break
			}
		}
		if c.CertDays < 0 {
			threshold = 0
		}
		k := certKey(c.ID, threshold)
		if v, _ := set.Get(ctx, k, ""); v != "" {
			continue
		}
		name := label(ctx, c.TargetEntity, c.TargetID)
		msg := fmt.Sprintf("TLS certificate for %s expires in %d days (%s)", name, days, c.CertIssuer)
		if days < 0 {
			msg = fmt.Sprintf("TLS certificate for %s expired %d days ago (%s)", name, -days, c.CertIssuer)
		}
		ev := alerts.Event{
			Entity: "certificate", TargetID: c.ID, Label: name, Kind: "tls",
			State: "warning", Message: msg, At: now,
		}
		if err := notifier.Broadcast(ctx, ev); err != nil {
			return sent, err
		}
		_ = set.Set(ctx, "system:expiry", k, now.UTC().Format("2006-01-02"))
		sent++
	}
	return sent, nil
}

func certKey(id int64, threshold int) string {
	return fmt.Sprintf("cert:%d:%d", id, threshold)
}

// Start runs domain and certificate checks at startup and every six hours.
// The returned stop waits.
func Start(ctx context.Context, inv *inventory.Store, set *settings.Store, notifier Notifier, chk *checks.Store, label func(context.Context, string, int64) string) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		run := func() {
			now := time.Now().UTC()
			if _, err := Run(ctx, inv, set, notifier, now); err != nil {
				fmt.Printf("expiry alerts: %v\n", err)
			}
			if chk != nil && label != nil {
				if _, err := RunCerts(ctx, chk, label, set, notifier, now); err != nil {
					fmt.Printf("certificate alerts: %v\n", err)
				}
			}
		}
		run()
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
	return func() { cancel(); <-done }
}
