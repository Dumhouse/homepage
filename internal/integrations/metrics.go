package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Monitoring tools already know every machine's OS, disk and uptime. A sync
// writes those as host facts right away and proposes hosts for machines that
// aren't in the inventory yet.

const CatMetrics = "Metrics"

// factsOp is an observation about a host, written without review.
func factsOp(host, address string, kv map[string]string) Op {
	d := map[string]string{"host": host, "address": address}
	for k, v := range kv {
		if v != "" || k == "error" { // an empty error clears an old warning
			d[k] = v
		}
	}
	return Op{Action: "update", Entity: "facts", Ref: host, Data: d}
}

func hostOp(name, address, kind, os, summary, source string) Op {
	return Op{Action: "create", Entity: "host", Ref: name, Note: summary,
		Data: map[string]string{"kind": kind, "address": address, "os": os, "summary": summary, "tags": source}}
}

// ---- Prometheus (node_exporter) ----

func Prometheus() *Plugin {
	return &Plugin{
		ID: "prometheus", Name: "Prometheus", Category: CatMetrics,
		Description: "Read node_exporter metrics: OS, kernel, root disk use and uptime of every scraped machine become host facts; unknown machines are proposed as hosts.",
		Fields: []Field{
			{Key: "url", Label: "Prometheus URL", Kind: "url", Required: true, Help: "e.g. http://192.168.1.30:9090 (VictoriaMetrics and Thanos query URLs work too)."},
			{Key: "token", Label: "Bearer token", Kind: "password", Secret: true, Help: "Only if Prometheus sits behind auth."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := promQuery(ctx, cfg, "up")
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			uname, err := promQuery(ctx, cfg, "node_uname_info")
			if err != nil {
				return nil, err
			}
			if len(uname) == 0 {
				return []Op{}, fmt.Errorf("Prometheus answers but has no node_exporter metrics (node_uname_info) — scrape node_exporter on your hosts")
			}
			byInst := func(q string) map[string]promSample {
				m := map[string]promSample{}
				list, _ := promQuery(ctx, cfg, q)
				for _, s := range list {
					m[s.Metric["instance"]] = s
				}
				return m
			}
			osInfo := byInst("node_os_info")
			boot := byInst("time() - node_boot_time_seconds")
			disk := byInst(`100 - 100 * node_filesystem_avail_bytes{mountpoint="/",fstype!="rootfs"} / node_filesystem_size_bytes{mountpoint="/",fstype!="rootfs"}`)
			reboot := byInst("node_reboot_required")
			ops := []Op{}
			for _, u := range uname {
				inst := u.Metric["instance"]
				addr := strings.TrimSuffix(strings.SplitN(inst, ":", 2)[0], "]")
				name := u.Metric["nodename"]
				osName := osInfo[inst].Metric["pretty_name"]
				if osName == "" {
					osName = u.Metric["sysname"]
				}
				kv := map[string]string{"os": osName, "kernel": u.Metric["release"], "hostname": name}
				if s, ok := boot[inst]; ok {
					kv["uptime"] = humanUptime(s.Value())
				}
				if s, ok := disk[inst]; ok {
					kv["disk_used_pct"] = strconv.Itoa(int(s.Value() + 0.5))
				}
				if s, ok := reboot[inst]; ok {
					kv["reboot_required"] = map[bool]string{true: "yes", false: "no"}[s.Value() > 0]
				}
				ops = append(ops, hostOp(name, addr, "bare", osName, "seen by Prometheus ("+u.Metric["job"]+")", "prometheus"),
					factsOp(name, addr, kv))
			}
			return ops, nil
		},
	}
}

type promSample struct {
	Metric map[string]string `json:"metric"`
	Val    []any             `json:"value"`
}

func (s promSample) Value() float64 {
	if len(s.Val) < 2 {
		return 0
	}
	str, _ := s.Val[1].(string)
	v, _ := strconv.ParseFloat(str, 64)
	return v
}

func promQuery(ctx context.Context, cfg Config, q string) ([]promSample, error) {
	var res struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []promSample `json:"result"`
		} `json:"data"`
	}
	h := map[string]string{}
	if t := strings.TrimSpace(cfg["token"]); t != "" {
		h["Authorization"] = "Bearer " + t
	}
	if err := getJSON(ctx, req{URL: baseURL(cfg["url"]) + "/api/v1/query?query=" + url.QueryEscape(q), What: "Prometheus", Header: h}, &res); err != nil {
		return nil, err
	}
	if res.Status != "success" {
		return nil, fmt.Errorf("Prometheus: %s", res.Error)
	}
	return res.Data.Result, nil
}

// ---- Netdata ----

func Netdata() *Plugin {
	return &Plugin{
		ID: "netdata", Name: "Netdata", Category: CatMetrics,
		Description: "Read OS, kernel, root disk use and uptime from Netdata agents. Point it at a parent to get all its children in one go.",
		Fields: []Field{
			{Key: "urls", Label: "Agent or parent URLs", Kind: "text", Required: true, Help: "Comma-separated, e.g. http://192.168.1.30:19999. A parent also reports its streaming children."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			for _, u := range splitList(cfg["urls"]) {
				if _, err := netdataInfo(ctx, baseURL(u)); err != nil {
					return err
				}
			}
			return nil
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			ops := []Op{}
			var firstErr error
			for _, raw := range splitList(cfg["urls"]) {
				base := baseURL(raw)
				info, err := netdataInfo(ctx, base)
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				hosts := info.MirroredHosts
				if len(hosts) == 0 {
					hosts = []string{""}
				}
				for i, h := range hosts {
					prefix, addr := base, ""
					if i == 0 {
						addr = urlHost(raw) // the agent itself
					} else {
						prefix = base + "/host/" + url.PathEscape(h)
					}
					hi := info
					if i > 0 {
						if c, err := netdataInfo(ctx, prefix); err == nil {
							hi = c
						}
					}
					name := h
					if name == "" {
						name = urlHost(raw)
					}
					osName := strings.TrimSpace(hi.OSName + " " + hi.OSVersion)
					kv := map[string]string{"os": osName, "kernel": hi.KernelVersion, "hostname": name}
					if v, ok := netdataLast(ctx, prefix, "system.uptime"); ok && len(v) > 0 {
						kv["uptime"] = humanUptime(v[0])
					}
					if v, ok := netdataLast(ctx, prefix, "disk_space./"); ok && len(v) >= 2 {
						total := 0.0
						for _, x := range v {
							total += x
						}
						kv["disk_used_pct"] = pct(v[1], total) // avail, used, reserved
					}
					ops = append(ops, hostOp(name, addr, "bare", osName, "seen by Netdata", "netdata"), factsOp(name, addr, kv))
				}
			}
			if len(ops) == 0 {
				return nil, firstErr
			}
			return ops, firstErr
		},
	}
}

type netdataAgent struct {
	OSName        string   `json:"os_name"`
	OSVersion     string   `json:"os_version"`
	KernelVersion string   `json:"kernel_version"`
	MirroredHosts []string `json:"mirrored_hosts"`
}

func netdataInfo(ctx context.Context, base string) (netdataAgent, error) {
	var i netdataAgent
	err := getJSON(ctx, req{URL: base + "/api/v1/info", What: "Netdata"}, &i)
	return i, err
}

// netdataLast returns the dimension values of the newest point of a chart.
func netdataLast(ctx context.Context, base, chart string) ([]float64, bool) {
	var res struct {
		Data [][]float64 `json:"data"`
	}
	if err := getJSON(ctx, req{URL: base + "/api/v1/data?chart=" + url.QueryEscape(chart) + "&after=-1&points=1&format=json", What: "Netdata"}, &res); err != nil || len(res.Data) == 0 || len(res.Data[0]) < 2 {
		return nil, false
	}
	return res.Data[0][1:], true
}

// ---- Beszel ----

func Beszel() *Plugin {
	return &Plugin{
		ID: "beszel", Name: "Beszel", Category: CatMetrics,
		Description: "Read every system Beszel monitors: hostname, kernel, disk use and uptime become host facts; unknown systems are proposed as hosts.",
		Fields: []Field{
			{Key: "url", Label: "Beszel hub URL", Kind: "url", Required: true, Help: "e.g. http://192.168.1.30:8090"},
			{Key: "email", Label: "User e-mail", Kind: "text", Required: true, Help: "A Beszel user; a read-only one is enough."},
			{Key: "password", Label: "Password", Kind: "password", Secret: true, Required: true},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := beszelSystems(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			systems, err := beszelSystems(ctx, cfg)
			if err != nil {
				return nil, err
			}
			ops := []Op{}
			for _, s := range systems {
				kv := map[string]string{"hostname": s.Info.Hostname, "kernel": s.Info.Kernel, "uptime": humanUptime(s.Info.Uptime)}
				if s.Info.DiskPct > 0 {
					kv["disk_used_pct"] = strconv.Itoa(int(s.Info.DiskPct + 0.5))
				}
				if s.Status == "down" {
					kv["error"] = "Beszel reports this system as down"
				}
				summary := "seen by Beszel"
				if s.Info.CPU != "" {
					summary += fmt.Sprintf(" · %s, %d cores", s.Info.CPU, s.Info.Cores)
				}
				ops = append(ops, hostOp(s.Name, s.Host, "bare", "", summary, "beszel"), factsOp(s.Name, s.Host, kv))
			}
			return ops, nil
		},
	}
}

type beszelSystem struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	Status string `json:"status"`
	Info   struct {
		Hostname string  `json:"h"`
		Kernel   string  `json:"k"`
		Uptime   float64 `json:"u"`
		DiskPct  float64 `json:"dp"`
		CPU      string  `json:"m"`
		Cores    int     `json:"c"`
	} `json:"info"`
}

func beszelSystems(ctx context.Context, cfg Config) ([]beszelSystem, error) {
	base := baseURL(cfg["url"])
	body, _ := json.Marshal(map[string]string{"identity": strings.TrimSpace(cfg["email"]), "password": cfg["password"]})
	var auth struct {
		Token string `json:"token"`
	}
	if err := getJSON(ctx, req{Method: "POST", URL: base + "/api/collections/users/auth-with-password", What: "Beszel login",
		Body: bytes.NewReader(body), Header: map[string]string{"Content-Type": "application/json"}}, &auth); err != nil {
		return nil, err
	}
	var res struct {
		Items []beszelSystem `json:"items"`
	}
	if err := getJSON(ctx, req{URL: base + "/api/collections/systems/records?perPage=500", What: "Beszel",
		Header: map[string]string{"Authorization": auth.Token}}, &res); err != nil {
		return nil, err
	}
	return res.Items, nil
}

// ---- Glances ----

func Glances() *Plugin {
	return &Plugin{
		ID: "glances", Name: "Glances", Category: CatMetrics,
		Description: "Read OS, kernel, root disk use and uptime from Glances web servers (glances -w) on your machines.",
		Fields: []Field{
			{Key: "urls", Label: "Glances URLs", Kind: "text", Required: true, Help: "Comma-separated, e.g. http://192.168.1.240:61208, http://192.168.1.241:61208"},
		},
		Test: func(ctx context.Context, cfg Config) error {
			for _, u := range splitList(cfg["urls"]) {
				if _, _, err := glancesAPI(ctx, baseURL(u), "system"); err != nil {
					return err
				}
			}
			return nil
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			ops := []Op{}
			var firstErr error
			for _, raw := range splitList(cfg["urls"]) {
				base := baseURL(raw)
				sysRaw, api, err := glancesAPI(ctx, base, "system")
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				var sys struct {
					Hostname    string `json:"hostname"`
					OSName      string `json:"os_name"`
					OSVersion   string `json:"os_version"`
					LinuxDistro string `json:"linux_distro"`
				}
				_ = json.Unmarshal(sysRaw, &sys)
				osName := sys.LinuxDistro
				if osName == "" {
					osName = sys.OSName
				}
				kv := map[string]string{"os": osName, "kernel": sys.OSVersion, "hostname": sys.Hostname}
				var up string
				if b, err := getRaw(ctx, base+api+"/uptime"); err == nil {
					_ = json.Unmarshal(b, &up)
					kv["uptime"] = strings.SplitN(up, ",", 2)[0]
				}
				var fs []struct {
					Mount   string  `json:"mnt_point"`
					Percent float64 `json:"percent"`
				}
				if b, err := getRaw(ctx, base+api+"/fs"); err == nil && json.Unmarshal(b, &fs) == nil {
					for _, f := range fs {
						if f.Mount == "/" {
							kv["disk_used_pct"] = strconv.Itoa(int(f.Percent + 0.5))
						}
					}
				}
				addr := urlHost(raw)
				ops = append(ops, hostOp(sys.Hostname, addr, "bare", osName, "seen by Glances", "glances"), factsOp(sys.Hostname, addr, kv))
			}
			if len(ops) == 0 {
				return nil, firstErr
			}
			return ops, firstErr
		},
	}
}

// glancesAPI fetches an endpoint from API v4, falling back to v3.
func glancesAPI(ctx context.Context, base, ep string) ([]byte, string, error) {
	var lastErr error
	for _, api := range []string{"/api/4", "/api/3"} {
		b, err := getRaw(ctx, base+api+"/"+ep)
		if err == nil {
			return b, api, nil
		}
		lastErr = err
	}
	return nil, "", fmt.Errorf("Glances: %w", lastErr)
}

func getRaw(ctx context.Context, u string) ([]byte, error) {
	var raw json.RawMessage
	err := getJSON(ctx, req{URL: u, What: "Glances"}, &raw)
	return raw, err
}
