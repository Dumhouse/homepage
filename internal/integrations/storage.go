package integrations

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Storage and hardware: NAS boxes, UPSes and server BMCs. Each becomes a
// host with its services; health problems (a degraded pool, a UPS on
// battery, a failing BMC sensor) show as a facts warning on the host.

const CatStorage = "Storage & hardware"

// ---- TrueNAS ----

func TrueNAS() *Plugin {
	return &Plugin{
		ID: "truenas", Name: "TrueNAS", Category: CatStorage,
		Description: "Import a TrueNAS box: version, RAM, pools and their health, and the sharing services it runs (SMB, NFS, iSCSI, SSH).",
		Fields: []Field{
			{Key: "url", Label: "TrueNAS URL", Kind: "url", Required: true, Help: "e.g. https://192.168.1.20"},
			{Key: "token", Label: "API key", Kind: "password", Secret: true, Required: true, Help: "Top right user menu → API Keys → Add. Use a read-only user where your version allows it."},
			{Key: "insecure", Label: "Accept self-signed certificate", Kind: "bool", Default: "1"},
		},
		Test: func(ctx context.Context, cfg Config) error {
			var info map[string]any
			return tnGet(ctx, cfg, "/system/info", &info)
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			var info struct {
				Version  string  `json:"version"`
				Hostname string  `json:"hostname"`
				Physmem  float64 `json:"physmem"`
				Uptime   float64 `json:"uptime_seconds"`
				Model    string  `json:"system_product"`
			}
			if err := tnGet(ctx, cfg, "/system/info", &info); err != nil {
				return nil, err
			}
			var pools []struct {
				Name    string `json:"name"`
				Status  string `json:"status"`
				Healthy bool   `json:"healthy"`
			}
			_ = tnGet(ctx, cfg, "/pool", &pools)
			var svcs []struct {
				Service string `json:"service"`
				State   string `json:"state"`
			}
			_ = tnGet(ctx, cfg, "/service", &svcs)

			addr := urlHost(cfg["url"])
			name := info.Hostname
			if name == "" {
				name = addr
			}
			poolText, bad := []string{}, []string{}
			for _, p := range pools {
				poolText = append(poolText, p.Name+" ("+p.Status+")")
				if !p.Healthy || p.Status != "ONLINE" {
					bad = append(bad, "pool "+p.Name+" is "+p.Status)
				}
			}
			summary := fmt.Sprintf("TrueNAS, %s GB RAM", gib(info.Physmem))
			if len(poolText) > 0 {
				summary += " · pools: " + strings.Join(poolText, ", ")
			}
			ops := []Op{hostOp(name, addr, "bare", info.Version, summary, "truenas,nas")}
			ports := map[string][2]string{"cifs": {"SMB", "445"}, "nfs": {"NFS", "2049"}, "iscsitarget": {"iSCSI", "3260"}, "ssh": {"SSH", "22"}, "ftp": {"FTP", "21"}, "webdav": {"WebDAV", "8080"}}
			u, _ := url.Parse(baseURL(cfg["url"]))
			webPort := u.Port()
			if webPort == "" {
				webPort = map[string]string{"https": "443", "http": "80"}[u.Scheme]
			}
			ops = append(ops, serviceOp(name, "TrueNAS UI", webPort, u.Scheme))
			for _, s := range svcs {
				if p, ok := ports[s.Service]; ok && s.State == "RUNNING" {
					ops = append(ops, serviceOp(name, p[0], p[1], "tcp"))
				}
			}
			ops = append(ops, factsOp(name, addr, map[string]string{"os": info.Version, "hostname": info.Hostname,
				"uptime": humanUptime(info.Uptime), "error": strings.Join(bad, "; ")}))
			return ops, nil
		},
	}
}

func tnGet(ctx context.Context, cfg Config, path string, out any) error {
	return getJSON(ctx, req{URL: baseURL(cfg["url"]) + "/api/v2.0" + path, What: "TrueNAS", Insecure: truthy(cfg["insecure"]),
		Header: map[string]string{"Authorization": "Bearer " + strings.TrimSpace(cfg["token"])}}, out)
}

func serviceOp(host, name, port, proto string) Op {
	return Op{Action: "create", Entity: "service", Ref: name, Note: name + " on " + host + ":" + port,
		Data: map[string]string{"host": host, "port": port, "protocol": proto}}
}

// ---- Synology ----

func Synology() *Plugin {
	return &Plugin{
		ID: "synology", Name: "Synology DSM", Category: CatStorage,
		Description: "Import a Synology NAS: model, DSM version, RAM, volumes with their use and health.",
		Fields: []Field{
			{Key: "url", Label: "DSM URL", Kind: "url", Required: true, Help: "e.g. https://192.168.1.21:5001"},
			{Key: "user", Label: "User", Kind: "text", Required: true, Help: "Make a separate DSM user; it needs no file access. Accounts with 2-factor login don't work over the API."},
			{Key: "password", Label: "Password", Kind: "password", Secret: true, Required: true},
			{Key: "insecure", Label: "Accept self-signed certificate", Kind: "bool", Default: "1"},
		},
		Test: func(ctx context.Context, cfg Config) error {
			sid, err := synoLogin(ctx, cfg)
			if err == nil {
				synoLogout(ctx, cfg, sid)
			}
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			sid, err := synoLogin(ctx, cfg)
			if err != nil {
				return nil, err
			}
			defer synoLogout(ctx, cfg, sid)
			var info struct {
				Model   string  `json:"model"`
				RAM     float64 `json:"ram"`
				Version string  `json:"version_string"`
				Uptime  float64 `json:"uptime"`
			}
			if err := synoCall(ctx, cfg, url.Values{"api": {"SYNO.DSM.Info"}, "version": {"2"}, "method": {"getinfo"}, "_sid": {sid}}, &info); err != nil {
				return nil, err
			}
			var st struct {
				Volumes []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
					Size   struct {
						Total string `json:"total"`
						Used  string `json:"used"`
					} `json:"size"`
				} `json:"volumes"`
			}
			_ = synoCall(ctx, cfg, url.Values{"api": {"SYNO.Storage.CGI.Storage"}, "version": {"1"}, "method": {"load_info"}, "_sid": {sid}}, &st)
			addr := urlHost(cfg["url"])
			name := "synology-" + strings.ToLower(strings.ReplaceAll(info.Model, " ", ""))
			vols, bad, maxPct := []string{}, []string{}, 0
			for _, v := range st.Volumes {
				total, _ := strconv.ParseFloat(v.Size.Total, 64)
				used, _ := strconv.ParseFloat(v.Size.Used, 64)
				p, _ := strconv.Atoi(pct(used, total))
				maxPct = max(maxPct, p)
				vols = append(vols, fmt.Sprintf("%s %d%% of %s GB", v.ID, p, gib(total)))
				if v.Status != "normal" && v.Status != "" {
					bad = append(bad, v.ID+" is "+v.Status)
				}
			}
			summary := fmt.Sprintf("Synology %s, %.0f MB RAM", info.Model, info.RAM)
			if len(vols) > 0 {
				summary += " · " + strings.Join(vols, ", ")
			}
			osName := "DSM " + info.Version
			kv := map[string]string{"os": osName, "uptime": humanUptime(info.Uptime), "error": strings.Join(bad, "; ")}
			if len(st.Volumes) > 0 {
				kv["disk_used_pct"] = strconv.Itoa(maxPct)
			}
			u, _ := url.Parse(baseURL(cfg["url"]))
			port := u.Port()
			if port == "" {
				port = map[string]string{"https": "443", "http": "80"}[u.Scheme]
			}
			return []Op{hostOp(name, addr, "bare", osName, summary, "synology,nas"), serviceOp(name, "DSM", port, u.Scheme),
				factsOp(name, addr, kv)}, nil
		},
	}
}

func synoCall(ctx context.Context, cfg Config, v url.Values, out any) error {
	var res struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	err := getJSON(ctx, req{Method: "POST", URL: baseURL(cfg["url"]) + "/webapi/entry.cgi", What: "Synology", Insecure: truthy(cfg["insecure"]),
		Body: strings.NewReader(v.Encode()), Header: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}}, &res)
	if err != nil {
		return err
	}
	if !res.Success {
		hint := map[int]string{400: "wrong user or password", 401: "account disabled", 402: "no permission", 403: "2-factor login is on for this account", 407: "blocked after failed logins"}[res.Error.Code]
		return fmt.Errorf("Synology %s: error %d %s", v.Get("api"), res.Error.Code, hint)
	}
	if out != nil {
		return json.Unmarshal(res.Data, out)
	}
	return nil
}

func synoLogin(ctx context.Context, cfg Config) (string, error) {
	var d struct {
		SID string `json:"sid"`
	}
	err := synoCall(ctx, cfg, url.Values{"api": {"SYNO.API.Auth"}, "version": {"6"}, "method": {"login"},
		"account": {cfg["user"]}, "passwd": {cfg["password"]}, "session": {"HomeServices"}, "format": {"sid"}}, &d)
	return d.SID, err
}

func synoLogout(ctx context.Context, cfg Config, sid string) {
	_ = synoCall(ctx, cfg, url.Values{"api": {"SYNO.API.Auth"}, "version": {"6"}, "method": {"logout"}, "session": {"HomeServices"}, "_sid": {sid}}, nil)
}

// ---- NUT (UPS) ----

func NUT() *Plugin {
	return &Plugin{
		ID: "nut", Name: "NUT (UPS)", Category: CatStorage,
		Description: "Read your UPSes from a Network UPS Tools server: model, battery, load and runtime. A UPS on battery or with a low battery shows as a warning.",
		Fields: []Field{
			{Key: "address", Label: "NUT server", Kind: "text", Required: true, Help: "host:port of upsd, e.g. 192.168.1.10:3493. upsd must listen on the LAN (LISTEN 0.0.0.0 in upsd.conf)."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := nutList(ctx, cfg["address"])
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			upses, err := nutList(ctx, cfg["address"])
			if err != nil {
				return nil, err
			}
			ops := []Op{}
			for name, v := range upses {
				model := strings.TrimSpace(v["device.mfr"] + " " + v["device.model"])
				if model == "" {
					model = strings.TrimSpace(v["ups.mfr"] + " " + v["ups.model"])
				}
				status := v["ups.status"]
				summary := fmt.Sprintf("UPS %s · battery %s%% · load %s%%", model, v["battery.charge"], v["ups.load"])
				if r := v["battery.runtime"]; r != "" {
					if sec, err := strconv.ParseFloat(r, 64); err == nil {
						summary += fmt.Sprintf(" · %d min runtime", int(sec/60))
					}
				}
				if w := v["ups.realpower.nominal"]; w != "" {
					summary += " · " + w + " W max"
				}
				warn := []string{}
				for _, f := range strings.Fields(status) {
					switch f {
					case "OB":
						warn = append(warn, "on battery")
					case "LB":
						warn = append(warn, "battery low")
					case "RB":
						warn = append(warn, "battery needs replacing")
					case "OVER":
						warn = append(warn, "overloaded")
					}
				}
				host := "ups-" + name
				ops = append(ops, Op{Action: "create", Entity: "host", Ref: host, Note: summary,
					Data: map[string]string{"kind": "bare", "os": model, "summary": summary, "tags": "ups,nut"}},
					factsOp(host, "", map[string]string{"os": model, "error": strings.Join(warn, ", ")}))
			}
			return ops, nil
		},
	}
}

// nutList returns every UPS on the server with its variables.
func nutList(ctx context.Context, address string) (map[string]map[string]string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("give the NUT server as host:port")
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "3493")
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, explainNetErr("NUT", "tcp://"+address, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	rd := bufio.NewReader(conn)
	list := func(cmd string) ([][]string, error) {
		if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
			return nil, err
		}
		out := [][]string{}
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("NUT: %w", err)
			}
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "ERR"):
				return nil, fmt.Errorf("NUT: %s", line)
			case strings.HasPrefix(line, "BEGIN"):
			case strings.HasPrefix(line, "END"):
				return out, nil
			default:
				out = append(out, nutFields(line))
			}
		}
	}
	upsLines, err := list("LIST UPS")
	if err != nil {
		return nil, err
	}
	res := map[string]map[string]string{}
	for _, f := range upsLines {
		if len(f) < 2 {
			continue
		}
		vars, err := list("LIST VAR " + f[1])
		if err != nil {
			return nil, err
		}
		m := map[string]string{}
		for _, v := range vars {
			if len(v) >= 4 {
				m[v[2]] = v[3]
			}
		}
		res[f[1]] = m
	}
	return res, nil
}

// nutFields splits a NUT protocol line, keeping "quoted values" whole.
func nutFields(line string) []string {
	out, cur, quoted := []string{}, strings.Builder{}, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// ---- IPMI / Redfish ----

func Redfish() *Plugin {
	return &Plugin{
		ID: "redfish", Name: "IPMI / Redfish", Category: CatStorage,
		Description: "Read a server's management controller (iDRAC, iLO, Supermicro, XClarity, OpenBMC) over Redfish: model, CPUs, RAM, power state, current power draw and hardware health.",
		Fields: []Field{
			{Key: "url", Label: "BMC URL", Kind: "url", Required: true, Help: "The address of the iDRAC/iLO/IPMI interface, e.g. https://192.168.1.50"},
			{Key: "user", Label: "User", Kind: "text", Required: true, Help: "A read-only BMC user (role ReadOnly / Operator)."},
			{Key: "password", Label: "Password", Kind: "password", Secret: true, Required: true},
			{Key: "insecure", Label: "Accept self-signed certificate", Kind: "bool", Default: "1"},
		},
		Test: func(ctx context.Context, cfg Config) error {
			var root map[string]any
			return rfGet(ctx, cfg, "/redfish/v1/Systems", &root)
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			var col struct {
				Members []struct {
					ID string `json:"@odata.id"`
				} `json:"Members"`
			}
			if err := rfGet(ctx, cfg, "/redfish/v1/Systems", &col); err != nil {
				return nil, err
			}
			watts := redfishWatts(ctx, cfg)
			bmc := urlHost(cfg["url"])
			ops := []Op{}
			for _, m := range col.Members {
				var sys struct {
					Manufacturer string `json:"Manufacturer"`
					Model        string `json:"Model"`
					HostName     string `json:"HostName"`
					SerialNumber string `json:"SerialNumber"`
					PowerState   string `json:"PowerState"`
					BiosVersion  string `json:"BiosVersion"`
					Status       struct {
						Health string `json:"Health"`
					} `json:"Status"`
					Memory struct {
						GiB float64 `json:"TotalSystemMemoryGiB"`
					} `json:"MemorySummary"`
					CPU struct {
						Count int    `json:"Count"`
						Model string `json:"Model"`
					} `json:"ProcessorSummary"`
				}
				if err := rfGet(ctx, cfg, m.ID, &sys); err != nil {
					return ops, err
				}
				name := sys.HostName
				if name == "" {
					name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(sys.Manufacturer+"-"+sys.Model), " ", "-"))
				}
				summary := fmt.Sprintf("%s %s · %d× %s · %.0f GB RAM · BMC %s", sys.Manufacturer, sys.Model, sys.CPU.Count, sys.CPU.Model, sys.Memory.GiB, bmc)
				if sys.SerialNumber != "" {
					summary += " · S/N " + sys.SerialNumber
				}
				if watts > 0 {
					summary += fmt.Sprintf(" · drawing %.0f W", watts)
				}
				warn := []string{}
				if sys.Status.Health != "" && sys.Status.Health != "OK" {
					warn = append(warn, "BMC reports health "+sys.Status.Health)
				}
				if sys.PowerState != "" && sys.PowerState != "On" {
					warn = append(warn, "powered "+strings.ToLower(sys.PowerState))
				}
				ops = append(ops, Op{Action: "create", Entity: "host", Ref: name, Note: summary, Data: map[string]string{
					"kind": "bare", "summary": summary, "tags": "bmc", "address2": bmc, "address2_kind": "other",
					"status": map[bool]string{true: "stopped"}[sys.PowerState == "Off"],
				}}, Op{Action: "link", Entity: "address", Ref: name, Note: "BMC address of " + name,
					Data: map[string]string{"host": name, "address": bmc, "kind": "other"}},
					factsOp(name, bmc, map[string]string{"error": strings.Join(warn, "; ")}))
			}
			return ops, nil
		},
	}
}

// redfishWatts is the first chassis' current power draw, 0 when unknown.
func redfishWatts(ctx context.Context, cfg Config) float64 {
	var col struct {
		Members []struct {
			ID string `json:"@odata.id"`
		} `json:"Members"`
	}
	if rfGet(ctx, cfg, "/redfish/v1/Chassis", &col) != nil || len(col.Members) == 0 {
		return 0
	}
	var p struct {
		PowerControl []struct {
			Watts float64 `json:"PowerConsumedWatts"`
		} `json:"PowerControl"`
	}
	if rfGet(ctx, cfg, col.Members[0].ID+"/Power", &p) != nil || len(p.PowerControl) == 0 {
		return 0
	}
	return p.PowerControl[0].Watts
}

func rfGet(ctx context.Context, cfg Config, path string, out any) error {
	u, err := url.Parse(baseURL(cfg["url"]))
	if err != nil {
		return err
	}
	u.Path = path
	auth := base64.StdEncoding.EncodeToString([]byte(cfg["user"] + ":" + cfg["password"]))
	return getJSON(ctx, req{URL: u.String(), What: "Redfish", Insecure: truthy(cfg["insecure"]),
		Header: map[string]string{"Authorization": "Basic " + auth}}, out)
}
