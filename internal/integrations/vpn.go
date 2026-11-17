package integrations

import (
	"context"
	"strings"
)

// VPN mesh networks: each device's VPN address is added to the matching host
// (by name), so SSH and links can use it. Devices not in the inventory
// (phones, laptops) are skipped unless "Add missing devices" is on.

const CatVPN = "VPN"

// TailscaleAPI is the Tailscale API base (a variable for tests).
var TailscaleAPI = "https://api.tailscale.com/api/v2"

type vpnDevice struct {
	Name    string // short host name
	FQDN    string
	IP      string // first IPv4 in the mesh
	OS      string
	Tags    []string
	Online  bool
	Version string
}

func Tailscale() *Plugin {
	return &Plugin{
		ID: "tailscale", Name: "Tailscale", Category: CatVPN,
		Description: "Add each machine's Tailscale address (100.x) to the matching host, so SSH and links work from anywhere. Optionally add tailnet devices that aren't documented yet.",
		Fields: []Field{
			{Key: "tailnet", Label: "Tailnet", Kind: "text", Default: "-", Help: "Your tailnet name (e.g. example.com or tail1234.ts.net), or - for the default tailnet of the key."},
			{Key: "token", Label: "API access token", Kind: "password", Secret: true, Required: true, Help: "Admin console → Settings → Keys → Generate access token (tskey-api-…). Expires after up to 90 days; an OAuth client with devices:core:read also works as a token."},
			{Key: "add_missing", Label: "Add missing devices", Kind: "bool", Help: "Propose a host for tailnet devices with no host of the same name. Off: only add addresses to hosts you already have."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := tailscaleDevices(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			devs, err := tailscaleDevices(ctx, cfg)
			if err != nil {
				return nil, err
			}
			return vpnOps(devs, "tailscale", truthy(cfg["add_missing"])), nil
		},
	}
}

func tailscaleDevices(ctx context.Context, cfg Config) ([]vpnDevice, error) {
	tailnet := strings.TrimSpace(cfg["tailnet"])
	if tailnet == "" {
		tailnet = "-"
	}
	var res struct {
		Devices []struct {
			Hostname           string   `json:"hostname"`
			Name               string   `json:"name"`
			Addresses          []string `json:"addresses"`
			OS                 string   `json:"os"`
			Tags               []string `json:"tags"`
			ClientVersion      string   `json:"clientVersion"`
			ConnectedToControl *bool    `json:"connectedToControl"`
		} `json:"devices"`
	}
	if err := getJSON(ctx, req{URL: TailscaleAPI + "/tailnet/" + tailnet + "/devices", What: "Tailscale",
		Header: map[string]string{"Authorization": "Bearer " + strings.TrimSpace(cfg["token"])}}, &res); err != nil {
		return nil, err
	}
	out := []vpnDevice{}
	for _, d := range res.Devices {
		tags := []string{}
		for _, t := range d.Tags {
			tags = append(tags, strings.TrimPrefix(t, "tag:"))
		}
		out = append(out, vpnDevice{Name: d.Hostname, FQDN: d.Name, IP: firstIPv4(d.Addresses), OS: d.OS, Tags: tags,
			Online: d.ConnectedToControl == nil || *d.ConnectedToControl, Version: strings.SplitN(d.ClientVersion, "-", 2)[0]})
	}
	return out, nil
}

func Headscale() *Plugin {
	return &Plugin{
		ID: "headscale", Name: "Headscale", Category: CatVPN,
		Description: "Self-hosted Tailscale control server: add each node's mesh address to the matching host.",
		Fields: []Field{
			{Key: "url", Label: "Headscale URL", Kind: "url", Required: true, Help: "e.g. https://hs.example.com"},
			{Key: "token", Label: "API key", Kind: "password", Secret: true, Required: true, Help: "On the server: headscale apikeys create --expiration 365d"},
			{Key: "add_missing", Label: "Add missing devices", Kind: "bool", Help: "Propose a host for nodes with no host of the same name."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := headscaleNodes(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			nodes, err := headscaleNodes(ctx, cfg)
			if err != nil {
				return nil, err
			}
			return vpnOps(nodes, "headscale", truthy(cfg["add_missing"])), nil
		},
	}
}

func headscaleNodes(ctx context.Context, cfg Config) ([]vpnDevice, error) {
	type node struct {
		Name        string   `json:"name"`
		GivenName   string   `json:"givenName"`
		IPAddresses []string `json:"ipAddresses"`
		Online      bool     `json:"online"`
		ForcedTags  []string `json:"forcedTags"`
	}
	var res struct {
		Nodes    []node `json:"nodes"`
		Machines []node `json:"machines"` // before Headscale 0.23
	}
	h := map[string]string{"Authorization": "Bearer " + strings.TrimSpace(cfg["token"])}
	base := baseURL(cfg["url"])
	if err := getJSON(ctx, req{URL: base + "/api/v1/node", What: "Headscale", Header: h}, &res); err != nil {
		if err2 := getJSON(ctx, req{URL: base + "/api/v1/machine", What: "Headscale", Header: h}, &res); err2 != nil {
			return nil, err
		}
	}
	out := []vpnDevice{}
	for _, n := range append(res.Nodes, res.Machines...) {
		name := n.GivenName
		if name == "" {
			name = n.Name
		}
		tags := []string{}
		for _, t := range n.ForcedTags {
			tags = append(tags, strings.TrimPrefix(t, "tag:"))
		}
		out = append(out, vpnDevice{Name: name, IP: firstIPv4(n.IPAddresses), Online: n.Online, Tags: tags})
	}
	return out, nil
}

// vpnOps adds each device's mesh address to its host; with addMissing it
// also proposes hosts for undocumented devices.
func vpnOps(devs []vpnDevice, source string, addMissing bool) []Op {
	ops := []Op{}
	for _, d := range devs {
		if d.IP == "" || d.Name == "" {
			continue
		}
		if addMissing {
			ops = append(ops, Op{Action: "create", Entity: "host", Ref: d.Name, Note: source + " device " + d.Name,
				Data: map[string]string{"kind": "bare", "address": d.IP, "address_kind": "tailscale", "os": d.OS,
					"summary": source + " device" + ternaryS(d.FQDN != "", " "+d.FQDN, ""), "tags": strings.Join(append([]string{source}, d.Tags...), ",")}})
		}
		data := map[string]string{"host": d.Name, "address": d.IP, "kind": "tailscale"}
		if !addMissing {
			data["optional"] = "1"
		}
		ops = append(ops, Op{Action: "link", Entity: "address", Ref: d.Name, Note: source + " address of " + d.Name, Data: data})
	}
	return ops
}

func ternaryS(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}
