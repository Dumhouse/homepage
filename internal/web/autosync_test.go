package web

import (
	"context"
	"testing"
	"time"

	"github.com/Dumhouse/homepage/internal/changesets"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
)

func TestAutoSync(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	calls := 0
	m := integrations.NewManager(d, integrations.NewMemorySecrets())
	m.Register(&integrations.Plugin{ID: "fake", Name: "Fake",
		Fields: []integrations.Field{{Key: "url", Label: "URL", Kind: "url"}},
		Sync: func(ctx context.Context, c integrations.Config) ([]integrations.Op, error) {
			calls++
			return []integrations.Op{{Action: "create", Entity: "host", Ref: "nas-auto", Data: map[string]string{"name": "nas-auto", "address": "192.0.2.50"}}}, nil
		}})
	m.Register(&integrations.Plugin{ID: "quiet", Name: "Quiet", Test: func(context.Context, integrations.Config) error { return nil }})
	ctx := context.Background()
	cs := changesets.New(d, inventory.New(d))
	now := time.Now().UTC()

	// Off by default: nothing syncs even when configured.
	if err := m.Save(ctx, "t", "fake", map[string]string{"url": "http://x"}); err != nil {
		t.Fatal(err)
	}
	if got := autoSyncOnce(ctx, m, cs, now); len(got) != 0 {
		t.Fatalf("auto-sync off but synced %v", got)
	}
	// Plugins without Sync ignore the setting.
	if err := m.Save(ctx, "t", "quiet", map[string]string{"_sync": "daily"}); err != nil {
		t.Fatal(err)
	}
	// An unknown value switches it off rather than being stored.
	if err := m.Save(ctx, "t", "fake", map[string]string{"url": "http://x", "_sync": "hourly"}); err != nil {
		t.Fatal(err)
	}
	if got := autoSyncOnce(ctx, m, cs, now); len(got) != 0 {
		t.Fatalf("invalid period must mean off, synced %v", got)
	}

	if err := m.Save(ctx, "t", "fake", map[string]string{"url": "http://x", "_sync": "daily"}); err != nil {
		t.Fatal(err)
	}
	got := autoSyncOnce(ctx, m, cs, now)
	if len(got) != 1 || got[0] != "fake" || calls != 1 {
		t.Fatalf("first run: %v calls=%d", got, calls)
	}
	pending, _ := cs.List(ctx, "pending")
	if len(pending) != 1 || pending[0].Source != "plugin:fake" || pending[0].Actor != "system:auto-sync" {
		t.Fatalf("pending = %+v", pending)
	}
	// Not due again yet.
	if got := autoSyncOnce(ctx, m, cs, now.Add(time.Hour)); len(got) != 0 {
		t.Fatalf("synced before the period passed: %v", got)
	}
	// Due, but the earlier proposal is still waiting: skipped.
	if got := autoSyncOnce(ctx, m, cs, now.Add(25*time.Hour)); len(got) != 0 {
		t.Fatalf("stacked a second proposal: %v", got)
	}
	// After review it runs again.
	if err := cs.Reject(ctx, "t", pending[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := autoSyncOnce(ctx, m, cs, now.Add(25*time.Hour)); len(got) != 1 || calls != 2 {
		t.Fatalf("after review: %v calls=%d", got, calls)
	}
	st, _ := m.List(ctx)
	for _, s := range st {
		if s.ID == "fake" && (s.AutoSync != "daily" || !s.CanSync()) {
			t.Fatalf("state: %+v", s)
		}
		if s.ID == "quiet" && (s.AutoSync != "" || s.CanSync()) {
			t.Fatalf("quiet state: %+v", s)
		}
	}
}

func TestSyncInterval(t *testing.T) {
	for v, want := range map[string]time.Duration{"": 0, "6h": 6 * time.Hour, "daily": 24 * time.Hour, "weekly": 168 * time.Hour, "x": 0} {
		if got := integrations.SyncInterval(v); got != want {
			t.Fatalf("%q: %v", v, got)
		}
	}
}
