package integrations

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// CatDisks groups disk-health plugins.
const CatDisks = "Disk health"

// Scrutiny reads SMART results collected by Scrutiny and puts a warning on
// every host with a failing disk (or clears it when all disks pass).
func Scrutiny() *Plugin {
	return &Plugin{
		ID: "scrutiny", Name: "Scrutiny", Category: CatDisks,
		Description: "Read SMART disk health from Scrutiny. A failing disk shows as a warning on its host (Needs a look); healthy hosts get the warning cleared.",
		Fields: []Field{
			{Key: "url", Label: "Scrutiny URL", Kind: "url", Required: true, Help: "The web UI, e.g. http://192.168.1.20:8080"},
			{Key: "default_host", Label: "Host for collectors without a host id", Kind: "text", Help: "Scrutiny collectors started without --host-id report no host. Name the inventory host they run on (e.g. nas)."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := scrutinySummary(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			disks, err := scrutinySummary(ctx, cfg)
			if err != nil {
				return nil, err
			}
			type hostDisks struct {
				bad, all []string
			}
			by := map[string]*hostDisks{}
			missingHost := 0
			for _, d := range disks {
				host := d.Device.HostID
				if host == "" {
					host = strings.TrimSpace(cfg["default_host"])
				}
				if host == "" {
					missingHost++
					continue
				}
				if by[host] == nil {
					by[host] = &hostDisks{}
				}
				label := d.Device.DeviceName
				if d.Device.Model != "" {
					label += " (" + d.Device.Model + ")"
				}
				desc := label
				if d.Smart.Temp > 0 {
					desc += fmt.Sprintf(" %d°C", d.Smart.Temp)
				}
				if d.Smart.PowerOnHours > 0 {
					desc += fmt.Sprintf(" %d h", d.Smart.PowerOnHours)
				}
				by[host].all = append(by[host].all, desc)
				switch {
				case d.Device.Status&1 != 0:
					by[host].bad = append(by[host].bad, "disk "+label+" fails SMART")
				case d.Device.Status&2 != 0:
					by[host].bad = append(by[host].bad, "disk "+label+" is at risk (Scrutiny)")
				}
			}
			hosts := make([]string, 0, len(by))
			for h := range by {
				hosts = append(hosts, h)
			}
			sort.Strings(hosts)
			ops := []Op{}
			for _, h := range hosts {
				op := factsOp(h, "", map[string]string{"error": strings.Join(by[h].bad, "; ")})
				op.Note = h + ": " + strings.Join(by[h].all, ", ")
				ops = append(ops, op)
			}
			if missingHost > 0 && len(ops) == 0 {
				return ops, fmt.Errorf("Scrutiny has %d disks but none name a host — set “Host for collectors without a host id”, or start collectors with --host-id", missingHost)
			}
			return ops, nil
		},
	}
}

type scrutinyDisk struct {
	Device struct {
		DeviceName string `json:"device_name"`
		Model      string `json:"model_name"`
		HostID     string `json:"host_id"`
		Status     int    `json:"device_status"`
	} `json:"device"`
	Smart struct {
		Temp         int `json:"temp"`
		PowerOnHours int `json:"power_on_hours"`
	} `json:"smart"`
}

func scrutinySummary(ctx context.Context, cfg Config) ([]scrutinyDisk, error) {
	var res struct {
		Data struct {
			Summary map[string]scrutinyDisk `json:"summary"`
		} `json:"data"`
	}
	if err := getJSON(ctx, req{URL: baseURL(cfg["url"]) + "/api/summary", What: "Scrutiny"}, &res); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(res.Data.Summary))
	for k := range res.Data.Summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]scrutinyDisk, 0, len(keys))
	for _, k := range keys {
		out = append(out, res.Data.Summary[k])
	}
	return out, nil
}
