package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Caddy reads routes from the admin API and proposes domain links. It never
// edits the Caddy config.
func Caddy() *Plugin {
	return &Plugin{
		ID:          "caddy",
		Category:    CatNetwork,
		Name:        "Caddy",
		Description: "Read hostnames and upstreams from the Caddy admin API on the edge box.",
		Fields: []Field{
			{Key: "admin_url", Label: "Admin API URL", Kind: "url", Required: true, Default: "http://192.168.1.254:2019", Help: "Caddy's admin endpoint (read-only GET /config/)."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := fetchCaddy(ctx, cfg["admin_url"])
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			raw, err := fetchCaddy(ctx, cfg["admin_url"])
			if err != nil {
				return nil, err
			}
			self := ""
			if u, err := url.Parse(cfg["admin_url"]); err == nil {
				self = u.Hostname()
			}
			return caddyOps(raw, self), nil
		},
	}
}

func fetchCaddy(ctx context.Context, base string) (map[string]any, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil, fmt.Errorf("%w: admin_url is required", ErrInvalid)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/config/", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil, fmt.Errorf("nothing answers at %s. Caddy's admin API listens only on localhost:2019 by default — add `admin 0.0.0.0:2019` (and firewall it to this server) to the global options of the Caddyfile, or leave this integration unused", base)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("caddy: HTTP 403 — the admin API refuses remote requests; add this server's address to `admin { origins … }` in the Caddyfile")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("caddy: HTTP %d", resp.StatusCode)
	}
	var cfg map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("caddy config: %w", err)
	}
	return cfg, nil
}

// caddyOps turns routes into "this domain is served by addr:port" links.
// Upstreams on localhost belong to the Caddy box itself (self).
func caddyOps(cfg map[string]any, self string) []Op {
	out := []Op{}
	apps, _ := cfg["apps"].(map[string]any)
	httpApp, _ := apps["http"].(map[string]any)
	servers, _ := httpApp["servers"].(map[string]any)
	for name, srvRaw := range servers {
		srv, _ := srvRaw.(map[string]any)
		routes, _ := srv["routes"].([]any)
		for _, rRaw := range routes {
			route, _ := rRaw.(map[string]any)
			hosts := caddyHosts(route)
			dials := caddyDials(route)
			for _, host := range hosts {
				if strings.HasPrefix(host, "*.") || host == "" {
					continue
				}
				for _, dial := range dials {
					addr, port, err := net.SplitHostPort(dial)
					if err != nil {
						break
					}
					if addr == "localhost" || addr == "127.0.0.1" || addr == "::1" || addr == "" {
						addr = self
					}
					out = append(out, Op{
						Action: "link", Entity: "service", Ref: host,
						Data: map[string]string{"domain": host, "address": addr, "port": port, "server": name},
						Note: "Caddy serves " + host + " from " + dial,
					})
					break
				}
			}
		}
	}
	return out
}

func caddyHosts(route map[string]any) []string {
	out := []string{}
	matches, _ := route["match"].([]any)
	for _, mRaw := range matches {
		m, _ := mRaw.(map[string]any)
		hosts, _ := m["host"].([]any)
		for _, h := range hosts {
			if s, ok := h.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func caddyDials(v any) []string {
	out := []string{}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "dial" {
				if s, ok := val.(string); ok {
					out = append(out, s)
				}
				continue
			}
			out = append(out, caddyDials(val)...)
		}
	case []any:
		for _, item := range t {
			out = append(out, caddyDials(item)...)
		}
	}
	return out
}
