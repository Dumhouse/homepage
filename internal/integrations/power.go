package integrations

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// KindPower marks the electricity plugin: no sync, it feeds the power
// poller and the Costs page.
const KindPower = "power"

// Power is the electricity plugin: a price per kWh and, optionally, Home
// Assistant for live readings. Shelly and Tasmota plugs need no settings —
// their address is set per host.
func Power() *Plugin {
	return &Plugin{
		ID: "power", Name: "Electricity", Kind: KindPower,
		Description: "What your hosts draw from the wall and what it costs per month. Set watts per host by hand, or read them live from Home Assistant, a Shelly or a Tasmota plug (host page → Power).",
		Fields: []Field{
			{Key: "price_kwh", Label: "Price per kWh", Kind: "text", Required: true, Default: "0.25", Help: "In your default currency, e.g. 0.25. Used for the Electricity rows on the Costs page."},
			{Key: "ha_url", Label: "Home Assistant URL", Kind: "url", Help: "Optional, e.g. http://192.168.1.20:8123 — for hosts whose power comes from an HA sensor."},
			{Key: "ha_token", Label: "Home Assistant token", Kind: "password", Secret: true, Help: "A long-lived access token (your HA profile → Security). Read-only use: /api/states/<entity>."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			if _, err := ParsePrice(cfg["price_kwh"]); err != nil {
				return err
			}
			base := strings.TrimRight(strings.TrimSpace(cfg["ha_url"]), "/")
			if base == "" {
				return nil
			}
			if cfg["ha_token"] == "" {
				return errors.New("the Home Assistant URL is set but its token is empty")
			}
			ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/", nil)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+cfg["ha_token"])
			res, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
			if err != nil {
				return fmt.Errorf("home assistant: %w", err)
			}
			res.Body.Close()
			switch res.StatusCode {
			case http.StatusOK:
				return nil
			case http.StatusUnauthorized:
				return errors.New("the Home Assistant token was refused (HTTP 401) — create a new long-lived token")
			default:
				return fmt.Errorf("home assistant: HTTP %d", res.StatusCode)
			}
		},
	}
}

// ParsePrice reads a price like "0.25" or "0,25".
func ParsePrice(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
	if err != nil || v < 0 || v > 100 {
		return 0, fmt.Errorf("price per kWh must be a number like 0.25, got %q", s)
	}
	return v, nil
}

// Config returns an integration's settings including secrets, for internal
// consumers such as the power poller. Never expose it over the API.
func (m *Manager) Config(ctx context.Context, id string) (Config, error) {
	return m.mergedConfig(ctx, id)
}
