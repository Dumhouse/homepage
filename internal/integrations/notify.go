package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dawidof/home-services/internal/alerts"
)

// Notification plugins send alerts somewhere you will see them. They have no
// Sync; Test sends a test message. Which events go out is chosen per plugin.

// eventFields are the per-plugin switches for what to send.
var eventFields = []Field{
	{Key: "on_down", Label: "Something goes down", Kind: "bool", Default: "1", Help: "A service, public URL or SSH login stops answering (after the grace period)."},
	{Key: "on_up", Label: "It comes back", Kind: "bool", Default: "1"},
	{Key: "on_discovery", Label: "New device on the network", Kind: "bool", Default: "1", Help: "A discovery scan finds an address you haven't registered or ignored."},
	{Key: "on_reminders", Label: "Expiry reminders and weekly digest", Kind: "bool", Default: "1"},
}

// wants reports whether a plugin's config asks for this event.
func wants(cfg Config, e alerts.Event) bool {
	on := func(k string) bool { return cfg[k] != "0" && cfg[k] != "" }
	switch e.State {
	case "down":
		return on("on_down")
	case "up":
		return on("on_up")
	case "discovered":
		return on("on_discovery")
	case "test":
		return true
	default: // warning, digest
		return on("on_reminders")
	}
}

var notifyClient = &http.Client{Timeout: 8 * time.Second}

// Ntfy pushes to an ntfy topic (ntfy.sh or self-hosted).
func Ntfy() *Plugin {
	return &Plugin{
		ID: "ntfy", Name: "ntfy", Kind: KindNotify,
		Description: "Push notifications to your phone through an ntfy topic (ntfy.sh or your own server).",
		Fields: append([]Field{
			{Key: "url", Label: "Topic URL", Kind: "url", Required: true, Help: "e.g. https://ntfy.example.com/homelab — subscribe to the same topic in the ntfy app."},
			{Key: "token", Label: "Access token", Kind: "password", Secret: true, Help: "Only for protected topics (tk_…)."},
		}, eventFields...),
		Notify: func(ctx context.Context, cfg Config, e alerts.Event) error {
			return (&alerts.Ntfy{URL: cfg["url"], Token: cfg["token"], Client: notifyClient}).Send(ctx, e)
		},
	}
}

// Gotify posts to a Gotify server with an application token.
func Gotify() *Plugin {
	return &Plugin{
		ID: "gotify", Name: "Gotify", Kind: KindNotify,
		Description: "Push notifications through a self-hosted Gotify server.",
		Fields: append([]Field{
			{Key: "url", Label: "Server URL", Kind: "url", Required: true, Help: "e.g. https://gotify.example.com"},
			{Key: "token", Label: "Application token", Kind: "password", Secret: true, Help: "Create an application in Gotify and paste its token."},
		}, eventFields...),
		Notify: func(ctx context.Context, cfg Config, e alerts.Event) error {
			base := strings.TrimRight(strings.TrimSpace(cfg["url"]), "/")
			if base == "" || cfg["token"] == "" {
				return fmt.Errorf("%w: server URL and token are required", ErrInvalid)
			}
			prio := 5
			if e.State == "down" {
				prio = 8
			}
			body, _ := json.Marshal(map[string]any{"title": alerts.Subject(e), "message": orSubject(e), "priority": prio})
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/message?token="+url.QueryEscape(cfg["token"]), bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			return do(req, "gotify")
		},
	}
}

// Webhook posts each event as JSON (Home Assistant, n8n, your own script…).
func Webhook() *Plugin {
	return &Plugin{
		ID: "webhook", Name: "Webhook", Kind: KindNotify,
		Description: "POST every event as JSON to a URL — Home Assistant, n8n, a script of your own.",
		Fields: append([]Field{
			{Key: "url", Label: "URL", Kind: "url", Required: true},
		}, eventFields...),
		Notify: func(ctx context.Context, cfg Config, e alerts.Event) error {
			return (&alerts.Webhook{URL: cfg["url"], Client: notifyClient}).Send(ctx, e)
		},
	}
}

func orSubject(e alerts.Event) string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return alerts.Subject(e)
}

func do(req *http.Request, name string) error {
	resp, err := notifyClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: HTTP %d", name, resp.StatusCode)
	}
	return nil
}

// pluginNotifier adapts a configured notification plugin to alerts.Notifier.
type pluginNotifier struct {
	m  *Manager
	id string
}

func (n pluginNotifier) Name() string { return n.id }

func (n pluginNotifier) Send(ctx context.Context, e alerts.Event) error {
	p, err := n.m.plugin(n.id)
	if err != nil {
		return err
	}
	cfg, err := n.m.mergedConfig(ctx, n.id)
	if err != nil {
		return err
	}
	if !wants(cfg, e) {
		return nil
	}
	return p.Notify(ctx, cfg, e)
}

// Notifiers returns every notification plugin that has been set up (its URL
// is saved). Config is read at send time, so edits apply without a restart.
func (m *Manager) Notifiers(ctx context.Context) []alerts.Notifier {
	var out []alerts.Notifier
	for _, id := range m.order {
		p := m.plugins[id]
		if p.Kind != KindNotify {
			continue
		}
		r, err := m.load(ctx, id)
		if err != nil || !r.enabled {
			continue
		}
		cfg := map[string]string{}
		_ = json.Unmarshal([]byte(r.config), &cfg)
		if strings.TrimSpace(cfg["url"]) == "" {
			continue
		}
		out = append(out, pluginNotifier{m: m, id: id})
	}
	return out
}

func testEvent() alerts.Event {
	return alerts.Event{Entity: "test", Label: "Home Services", State: "test",
		Message: "Test notification — if you can read this, alerts will reach you here.", At: time.Now()}
}
