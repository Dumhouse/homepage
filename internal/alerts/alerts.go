// Package alerts delivers check state transitions to ntfy and webhooks.
// Rules live in Dispatcher: notify on down and on recovery, dedupe repeats and
// suppress recovery for very short outages (flap protection).
package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Event struct {
	CheckID  int64     `json:"check_id"`
	Entity   string    `json:"entity"`
	TargetID int64     `json:"target_id"`
	Label    string    `json:"label,omitempty"`
	Kind     string    `json:"kind"`
	State    string    `json:"state"` // down | up
	Previous string    `json:"previous,omitempty"`
	Message  string    `json:"message,omitempty"`
	At       time.Time `json:"at"`
}

type Notifier interface {
	Name() string
	Send(context.Context, Event) error
}

// Subject is the one-line title for an event.
func Subject(e Event) string { return subject(e) }

func subject(e Event) string {
	label := strings.TrimSpace(e.Label)
	if label == "" {
		label = fmt.Sprintf("%s #%d", e.Entity, e.TargetID)
	}
	switch e.State {
	case "down":
		return label + " is down"
	case "warning":
		return "check " + label
	case "digest", "test":
		return label
	case "discovered":
		return "New device on the network: " + label
	default:
		return label + " is back up"
	}
}

// Ntfy posts the message to a full topic URL (https://ntfy.sh/topic or self-hosted).
type Ntfy struct {
	URL    string
	Token  string
	Client *http.Client
}

func (n *Ntfy) Name() string { return "ntfy" }

func (n *Ntfy) Send(ctx context.Context, e Event) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, strings.NewReader(e.Message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", subject(e))
	priority, tags := "default", "white_check_mark"
	switch e.State {
	case "down":
		priority, tags = "high", "rotating_light"
	case "warning":
		tags = "warning"
	case "digest":
		tags = "calendar"
	case "discovered":
		tags = "mag"
	case "test":
		tags = "wave"
	}
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", tags)
	if n.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.Token)
	}
	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Webhook posts the event as JSON to a generic endpoint.
type Webhook struct {
	URL    string
	Client *http.Client
}

func (w *Webhook) Name() string { return "webhook" }

func (w *Webhook) Send(ctx context.Context, e Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "home-services-alerts/1")
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
	}
	return nil
}

// FromEnv builds notifiers from HS_NTFY_URL (+ HS_NTFY_TOKEN) and HS_WEBHOOK_URL.
func FromEnv() []Notifier {
	var out []Notifier
	if u := strings.TrimSpace(os.Getenv("HS_NTFY_URL")); u != "" {
		out = append(out, &Ntfy{URL: u, Token: os.Getenv("HS_NTFY_TOKEN")})
	}
	if u := strings.TrimSpace(os.Getenv("HS_WEBHOOK_URL")); u != "" {
		out = append(out, &Webhook{URL: u})
	}
	return out
}

// MinDownFromEnv reads HS_ALERTS_MIN_DOWN (seconds, default 120).
func MinDownFromEnv() time.Duration {
	if v := os.Getenv("HS_ALERTS_MIN_DOWN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 2 * time.Minute
}

// Dispatcher fans one event out to every notifier with dedupe and flap control.
type Dispatcher struct {
	Notifiers []Notifier
	MinDown   time.Duration
	Dedupe    time.Duration
	Now       func() time.Time

	// Source, when set, adds notifiers looked up at send time (the
	// notification plugins configured in the UI).
	Source func(context.Context) []Notifier

	mu        sync.Mutex
	downSince map[int64]time.Time
	sent      map[string]time.Time
}

func NewDispatcher(notifiers []Notifier, minDown time.Duration) *Dispatcher {
	return &Dispatcher{
		Notifiers: notifiers, MinDown: minDown, Dedupe: 10 * time.Minute,
		Now: time.Now, downSince: map[int64]time.Time{}, sent: map[string]time.Time{},
	}
}

func (d *Dispatcher) Name() string { return "dispatcher" }

// Send applies the rules and fans out. Recovery events shorter than MinDown are
// dropped; the same check+state is not re-sent within Dedupe.
func (d *Dispatcher) Send(ctx context.Context, e Event) error {
	d.mu.Lock()
	now := d.Now()
	switch e.State {
	case "down":
		d.downSince[e.CheckID] = now
	case "up":
		since, ok := d.downSince[e.CheckID]
		delete(d.downSince, e.CheckID)
		if !ok {
			d.mu.Unlock()
			return nil // never saw the outage
		}
		if now.Sub(since) < d.MinDown {
			d.mu.Unlock()
			return nil // flapped back before it mattered
		}
	case "warning":
		// warnings are one-off suggestions (e.g. SSH passwords enabled)
	default:
		d.mu.Unlock()
		return nil
	}
	key := fmt.Sprintf("%d:%s", e.CheckID, e.State)
	if t, ok := d.sent[key]; ok && now.Sub(t) < d.Dedupe {
		d.mu.Unlock()
		return nil
	}
	d.sent[key] = now
	d.mu.Unlock()

	return d.fanout(ctx, e)
}

// Broadcast sends an event as-is (no down/up rules), used by the weekly digest.
func (d *Dispatcher) Broadcast(ctx context.Context, e Event) error {
	return d.fanout(ctx, e)
}

func (d *Dispatcher) fanout(ctx context.Context, e Event) error {
	var errs []error
	all := append([]Notifier{}, d.Notifiers...)
	if d.Source != nil {
		all = append(all, d.Source(ctx)...)
	}
	for _, n := range all {
		sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := n.Send(sendCtx, e); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", n.Name(), err))
		}
		cancel()
	}
	return errors.Join(errs...)
}
