package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Cloudflare reads zones and DNS records and proposes domain/record ops.
// Read-only: a token with Zone:Read + DNS:Read is enough.
func Cloudflare() *Plugin {
	return &Plugin{
		ID:          "cloudflare",
		Name:        "Cloudflare",
		Description: "Read zones and DNS records; propose domains and record entries.",
		Fields: []Field{
			{Key: "api_token", Label: "API token", Kind: "password", Secret: true, Required: true, Help: "Zone:Read + DNS:Read."},
			{Key: "api_base", Label: "API base", Kind: "url", Default: "https://api.cloudflare.com/client/v4", Help: "Only change for tests or a proxy."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			var out struct {
				Status string `json:"status"`
			}
			if err := cfGet(ctx, cfg, "/user/tokens/verify", &out); err != nil {
				return err
			}
			if out.Status != "" && out.Status != "active" {
				return fmt.Errorf("cloudflare: token status %q", out.Status)
			}
			return nil
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			var zones []struct {
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				Status      string   `json:"status"`
				Nameservers []string `json:"name_servers"`
			}
			if err := cfGet(ctx, cfg, "/zones?per_page=50", &zones); err != nil {
				return nil, err
			}
			ops := []Op{}
			for _, z := range zones {
				if z.Name == "" {
					continue
				}
				ops = append(ops, Op{
					Action: "create", Entity: "domain", Ref: z.Name,
					Data: map[string]string{
						"source": "cloudflare", "zone_id": z.ID, "status": z.Status,
						"nameservers": strings.Join(z.Nameservers, "\n"),
					},
					Note: "Cloudflare zone",
				})
				var records []struct {
					Name    string `json:"name"`
					Type    string `json:"type"`
					Content string `json:"content"`
					Proxied bool   `json:"proxied"`
				}
				if err := cfGet(ctx, cfg, "/zones/"+url.PathEscape(z.ID)+"/dns_records?per_page=100", &records); err != nil {
					continue
				}
				for _, r := range records {
					rel := strings.TrimSuffix(strings.TrimSuffix(r.Name, z.Name), ".")
					if rel == "" {
						rel = "@"
					}
					ops = append(ops, Op{
						Action: "create", Entity: "dns_record", Ref: r.Name + " " + r.Type,
						Data: map[string]string{
							"domain": z.Name, "name": rel, "type": r.Type, "content": r.Content,
							"proxied": fmt.Sprintf("%t", r.Proxied),
						},
						Note: "Cloudflare record",
					})
				}
				if len(ops) > 1000 {
					return ops, nil
				}
			}
			return ops, nil
		},
	}
}

func cfGet(ctx context.Context, cfg Config, path string, out any) error {
	base := strings.TrimRight(strings.TrimSpace(cfg["api_base"]), "/")
	if base == "" {
		base = "https://api.cloudflare.com/client/v4"
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg["api_token"])
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var wrapper struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return fmt.Errorf("cloudflare: %w", err)
	}
	if resp.StatusCode != http.StatusOK || !wrapper.Success {
		msg := "HTTP " + resp.Status
		if len(wrapper.Errors) > 0 {
			msg = wrapper.Errors[0].Message
		}
		return fmt.Errorf("cloudflare: %s", msg)
	}
	return json.Unmarshal(wrapper.Result, out)
}
