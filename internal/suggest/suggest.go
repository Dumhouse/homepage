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
			for _, m := range byPort {
				if (m.port != 0 && v.Port == m.port) || (m.word != "" && strings.Contains(name, m.word)) {
					add(Suggestion{
						Plugin: m.plugin, Name: m.name, Reason: m.reason + " on " + h.Name,
						Href: "/settings/integrations?p=" + m.plugin, HostID: h.ID,
					})
				}
			}
		}
		for _, a := range full.Addresses {
			if a.Kind == "tailscale" {
				add(Suggestion{Plugin: "tailscale", Name: "Tailscale",
					Reason: h.Name + " has a Tailscale address — fill in the 100.x address of every other host automatically",
					Href:   "/settings/integrations?p=tailscale", HostID: h.ID})
			}
		}
		if h.Kind == "vps" {
			add(Suggestion{Plugin: "hetzner", Name: "Hetzner Cloud / DigitalOcean / OVH",
				Reason: h.Name + " is a VPS — import cloud servers with their addresses and monthly price",
				Href:   "/settings/integrations?p=hetzner", HostID: h.ID})
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

// byPort maps a well-known port (or a word in the service name) to the
// plugin that reads it.
var byPort = []struct {
	port               int
	word, plugin, name string
	reason             string
}{
	{9090, "prometheus", "prometheus", "Prometheus", "Prometheus answers"},
	{19999, "netdata", "netdata", "Netdata", "Netdata answers"},
	{8090, "beszel", "beszel", "Beszel", "Beszel hub"},
	{61208, "glances", "glances", "Glances", "Glances web server"},
	{3493, "nut", "nut", "NUT (UPS)", "UPS server (NUT) port 3493"},
	{5001, "synology", "synology", "Synology DSM", "Synology DSM port 5001"},
	{0, "truenas", "truenas", "TrueNAS", "TrueNAS"},
	{0, "headscale", "headscale", "Headscale", "Headscale"},
	{0, "idrac", "redfish", "IPMI / Redfish", "a server management controller"},
	{623, "ipmi", "redfish", "IPMI / Redfish", "IPMI port 623"},
	{8443, "unifi", "unifi", "UniFi Network", "UniFi controller"},
	{0, "opnsense", "opnsense", "OPNsense", "OPNsense"},
	{8291, "mikrotik", "mikrotik", "MikroTik RouterOS", "MikroTik (Winbox)"},
	{0, "openwrt", "openwrt", "OpenWrt", "OpenWrt"},
	{0, "pi-hole", "pihole", "Pi-hole", "Pi-hole"},
	{0, "pihole", "pihole", "Pi-hole", "Pi-hole"},
	{0, "adguard", "adguard", "AdGuard Home", "AdGuard Home"},
	{81, "nginx proxy manager", "npm", "Nginx Proxy Manager", "Nginx Proxy Manager admin port 81"},
	{0, "traefik", "traefik", "Traefik", "Traefik"},
	{0, "scrutiny", "scrutiny", "Scrutiny", "Scrutiny"},
}
