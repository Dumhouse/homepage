package alerts_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Dumhouse/homepage/internal/alerts"
)

type capture struct{ events []alerts.Event }

func (c *capture) Name() string { return "capture" }
func (c *capture) Send(ctx context.Context, e alerts.Event) error {
	c.events = append(c.events, e)
	return nil
}

func TestDispatcherRules(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)
	c := &capture{}
	d := alerts.NewDispatcher([]alerts.Notifier{c}, 2*time.Minute)
	d.Now = func() time.Time { return now }

	down := alerts.Event{CheckID: 1, Entity: "service", TargetID: 2, State: "down", Message: "boom"}
	up := alerts.Event{CheckID: 1, Entity: "service", TargetID: 2, State: "up", Message: "fine"}

	_ = d.Send(ctx, down)
	if len(c.events) != 1 {
		t.Fatalf("down should notify: %+v", c.events)
	}
	_ = d.Send(ctx, down)
	if len(c.events) != 1 {
		t.Errorf("repeated down should dedupe: %+v", c.events)
	}
	now = now.Add(30 * time.Second)
	_ = d.Send(ctx, up)
	if len(c.events) != 1 {
		t.Errorf("short outage should not notify recovery: %+v", c.events)
	}

	now = now.Add(12 * time.Minute)
	_ = d.Send(ctx, down)
	if len(c.events) != 2 {
		t.Fatalf("second outage should notify: %+v", c.events)
	}
	now = now.Add(5 * time.Minute)
	_ = d.Send(ctx, up)
	if len(c.events) != 3 || c.events[2].State != "up" {
		t.Errorf("real outage recovery should notify: %+v", c.events)
	}

	// an up with no outage on record is ignored
	_ = d.Send(ctx, up)
	if len(c.events) != 3 {
		t.Errorf("orphan recovery should not notify: %+v", c.events)
	}

	// broadcast (digest) bypasses the down/up rules
	before := len(c.events)
	if err := d.Broadcast(ctx, alerts.Event{Entity: "digest", State: "digest", Message: "summary"}); err != nil {
		t.Fatal(err)
	}
	if len(c.events) != before+1 || c.events[len(c.events)-1].State != "digest" {
		t.Errorf("broadcast: %+v", c.events)
	}

	// warnings go out once, then dedupe
	warning := alerts.Event{CheckID: 2, Entity: "ssh_access", TargetID: 3, State: "warning", Label: "ssh deploy on acme"}
	beforeWarning := len(c.events)
	_ = d.Send(ctx, warning)
	_ = d.Send(ctx, warning)
	if len(c.events) != beforeWarning+1 || c.events[beforeWarning].State != "warning" {
		t.Errorf("warning should send once: %+v", c.events)
	}
}

func TestNtfy(t *testing.T) {
	var title, tags, body, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		title, tags, auth = r.Header.Get("Title"), r.Header.Get("Tags"), r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	n := &alerts.Ntfy{URL: srv.URL, Token: "tok", Client: srv.Client()}
	err := n.Send(context.Background(), alerts.Event{State: "down", Label: "acme/web", Message: "connection refused"})
	if err != nil {
		t.Fatal(err)
	}
	if title != "acme/web is down" || tags != "rotating_light" || body != "connection refused" || auth != "Bearer tok" {
		t.Errorf("ntfy request: title=%q tags=%q auth=%q body=%q", title, tags, auth, body)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	if err := (&alerts.Ntfy{URL: bad.URL, Client: bad.Client()}).Send(context.Background(), alerts.Event{State: "up"}); err == nil {
		t.Error("non-2xx should be an error")
	}
}

func TestWebhook(t *testing.T) {
	var got alerts.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type: %q", r.Header.Get("Content-Type"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	in := alerts.Event{CheckID: 3, Entity: "host", TargetID: 4, State: "down", Message: "unreachable"}
	if err := (&alerts.Webhook{URL: srv.URL, Client: srv.Client()}).Send(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if got.CheckID != 3 || got.State != "down" || got.Message != "unreachable" {
		t.Errorf("webhook body: %+v", got)
	}
}
