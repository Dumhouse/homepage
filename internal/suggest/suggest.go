// Package suggest recommends read-only integrations based on what is already
// in the inventory: a Proxmox port, a Caddy admin port, domains, unknown watthours.
package suggest

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dawidof/home-services/internal/inventory"
)

type Suggestion struct {
	Plugin string `json:"plugin"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
	Href   string `json:"href,omitempty"`
	HostID int64  `json:"host_id,omitempty"`
}

// For returns one suggestion per plugin that is not already configured.
// configured is keyed by plugin id (saved integrations).
func For(ctx context.Context, inv *inventory.Store, configured map[string]bool) ([]Suggestion, error) {
	hosts, err := inv.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	out := []Suggestion{}
	add := func(s Suggestion) {
		if configured[s.Plugin] {
			return
		}
		for _, existing := range out {
			if existing.Plugin == s.Plugin {
				return
			}
		}
		out = append(out, s)
	}
	for _, h := range hosts {
		full, err := inv.GetHost(ctx, h.ID)
		if err != nil {
			continue
		}
		for _, v := range full.Services {
			name := strings.ToLower(v.Name + " " + v.Type + " " + v.Domain)
			switch {
			case v.Port == 8006 || strings.Contains(name, "proxmox"):
				add(Suggestion{
					Plugin: "proxmox", Name: "Proxmox",
					Reason: "Port 8006 on " + h.Name + " — import nodes, VMs and containers",
					Href:   "/settings/integrations?p=proxmox", HostID: h.ID,
				})
			case v.Port == 2019 || strings.Contains(name, "caddy"):
				add(Suggestion{
					Plugin: "caddy", Name: "Caddy",
					Reason: "Admin port 2019 on " + h.Name + " — import routes as service links",
					Href:   "/settings/integrations?p=caddy", HostID: h.ID,
				})
			}
		}
	}
	domains, err := inv.ListDomains(ctx)
	if err != nil {
		return nil, err
	}
	if len(domains) > 0 {
		add(Suggestion{
			Plugin: "cloudflare", Name: "Cloudflare",
			Reason: "You track " + plural(len(domains), "domain") + " — import zones and DNS records instead of typing them",
			Href:   "/settings/integrations?p=cloudflare",
		})
	}
	if len(hosts) > 0 {
		add(Suggestion{
			Plugin: "power", Name: "Electricity",
			Reason: "Set a price per kWh and watts per host to see electricity on Costs",
			Href:   "/settings/integrations?p=power",
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
