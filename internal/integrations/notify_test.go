package integrations_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Dumhouse/homepage/internal/alerts"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/integrations"
)

// A configured ntfy plugin receives alerts through the dispatcher, honours
// its per-event switches, and Test sends a test message.
func TestNtfyPluginNotifies(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.Header.Get("Title")+"|"+string(b))
		mu.Unlock()
	}))
	defer srv.Close()

	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	m := integrations.NewManager(d, integrations.NewMemorySecrets())
	m.Register(integrations.Ntfy())
	m.Register(integrations.Webhook())
	ctx := context.Background()
	if n := m.Notifiers(ctx); len(n) != 0 {
		t.Fatalf("nothing set up yet, got %d notifiers", len(n))
	}
	// on_up unchecked (absent from the form)
	if err := m.Save(ctx, "test", "ntfy", map[string]string{"url": srv.URL + "/lab", "on_down": "1", "on_discovery": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Test(ctx, "ntfy"); err != nil {
		t.Fatal(err)
	}
	disp := alerts.NewDispatcher(nil, 0)
	disp.Source = m.Notifiers
	_ = disp.Broadcast(ctx, alerts.Event{Label: "nas (10.0.0.4)", State: "discovered", Message: "Open ports: 22"})
	_ = disp.Broadcast(ctx, alerts.Event{Label: "web", State: "up"})
	_ = disp.Broadcast(ctx, alerts.Event{Label: "web", State: "down"})
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("want test + discovered + down (up is switched off), got %q", got)
	}
	if got[1] != "New device on the network: nas (10.0.0.4)|Open ports: 22" || got[2] != "web is down|" {
		t.Fatalf("messages: %q", got)
	}
	if _, err := m.Sync(ctx, "ntfy"); err == nil {
		t.Fatal("a notification plugin has nothing to sync")
	}
}
