package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ---- OpenMediaVault ----

// OpenMediaVault reads a NAS through the web UI's own RPC endpoint (/rpc.php),
// the same calls the OMV interface makes: version, RAM, uptime, filesystem use
// and SMART status. Use a read-only OMV account.
func OpenMediaVault() *Plugin {
	return &Plugin{
		ID: "omv", Name: "OpenMediaVault", Category: CatStorage,
		Description: "Import an OpenMediaVault NAS: version, uptime, RAM, how full each filesystem is, and SMART health. A failing disk or a nearly full filesystem shows as a warning on the host.",
		Fields: []Field{
			{Key: "url", Label: "OpenMediaVault URL", Kind: "url", Required: true, Help: "The web UI, e.g. http://192.168.1.20"},
			{Key: "user", Label: "User", Kind: "text", Required: true, Help: "An OMV web-UI user. admin works, but a separate account with the read-only (viewer) role is safer."},
			{Key: "password", Label: "Password", Kind: "password", Secret: true, Required: true},
			{Key: "insecure", Label: "Accept self-signed certificate", Kind: "bool", Default: "1"},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := omvLogin(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			cookie, err := omvLogin(ctx, cfg)
			if err != nil {
				return nil, err
			}
			var info struct {
				Hostname string  `json:"hostname"`
				Version  string  `json:"version"`
				Kernel   string  `json:"kernel"`
				Uptime   float64 `json:"uptime"`
				MemTotal float64 `json:"memTotal"`
				CPU      string  `json:"cpuModelName"`
			}
			if err := omvCall(ctx, cfg, cookie, "System", "getInformation", nil, &info); err != nil {
				return nil, err
			}
			var fs []struct {
				Device  string      `json:"devicefile"`
				Label   string      `json:"label"`
				Mount   string      `json:"mountpoint"`
				Size    json.Number `json:"size"`
				Used    json.Number `json:"used"`
				Percent json.Number `json:"percentage"`
			}
			_ = omvCall(ctx, cfg, cookie, "FileSystemMgmt", "enumerateMountedFilesystems", map[string]any{"includeroot": true}, &fs)
			var smart struct {
				Data []struct {
					Device string `json:"devicefile"`
					Model  string `json:"model"`
					Status string `json:"overallstatus"`
				} `json:"data"`
			}
			_ = omvCall(ctx, cfg, cookie, "Smart", "getList", map[string]any{"start": 0, "limit": -1}, &smart)

			addr := urlHost(cfg["url"])
			name := info.Hostname
			if name == "" {
				name = addr
			}
			maxPct, bad, vols := 0, []string{}, []string{}
			for _, f := range fs {
				p, _ := strconv.Atoi(strings.TrimSpace(f.Percent.String()))
				if f.Percent.String() == "" { // older versions only give sizes
					size, _ := f.Size.Float64()
					used, _ := f.Used.Float64()
					p, _ = strconv.Atoi(pct(used, size))
				}
				maxPct = max(maxPct, p)
				label := f.Label
				if label == "" {
					label = f.Device
				}
				vols = append(vols, fmt.Sprintf("%s %d%%", label, p))
				if p >= 95 {
					bad = append(bad, fmt.Sprintf("%s is %d%% full", label, p))
				}
			}
			for _, d := range smart.Data {
				if s := strings.ToUpper(d.Status); s != "" && s != "GOOD" && s != "OK" && s != "UNKNOWN" {
					bad = append(bad, fmt.Sprintf("%s (%s) SMART %s", d.Device, strings.TrimSpace(d.Model), d.Status))
				}
			}
			osName := "OpenMediaVault " + info.Version
			summary := "OpenMediaVault NAS"
			if info.MemTotal > 0 {
				summary += ", " + gib(info.MemTotal) + " GB RAM"
			}
			if len(vols) > 0 {
				summary += " · " + strings.Join(vols, ", ")
			}
			kv := map[string]string{"os": osName, "kernel": info.Kernel, "uptime": humanUptime(info.Uptime), "error": strings.Join(bad, "; ")}
			if len(fs) > 0 {
				kv["disk_used_pct"] = strconv.Itoa(maxPct)
			}
			u, _ := url.Parse(baseURL(cfg["url"]))
			port := u.Port()
			if port == "" {
				port = map[string]string{"https": "443", "http": "80"}[u.Scheme]
			}
			return []Op{hostOp(name, addr, "bare", osName, summary, "omv,nas"),
				serviceOp(name, "OpenMediaVault", port, u.Scheme),
				factsOp(name, addr, kv)}, nil
		},
	}
}

// omvLogin signs in and returns the session cookie.
func omvLogin(ctx context.Context, cfg Config) (string, error) {
	var out struct {
		Authenticated bool   `json:"authenticated"`
		Message       string `json:"errmsg"`
	}
	cookie, err := omvRPC(ctx, cfg, "", "Session", "login",
		map[string]any{"username": cfg["user"], "password": cfg["password"]}, &out)
	if err != nil {
		return "", err
	}
	if !out.Authenticated {
		return "", fmt.Errorf("OpenMediaVault refused the login — check the user and password in the web UI (%s)", strings.TrimSpace(out.Message))
	}
	return cookie, nil
}

func omvCall(ctx context.Context, cfg Config, cookie, service, method string, params any, out any) error {
	_, err := omvRPC(ctx, cfg, cookie, service, method, params, out)
	return err
}

// omvRPC posts one call to /rpc.php. It returns the session cookie the server
// set (only the login call sets one).
func omvRPC(ctx context.Context, cfg Config, cookie, service, method string, params any, out any) (string, error) {
	body, _ := json.Marshal(map[string]any{"service": service, "method": method, "params": params, "options": nil})
	endpoint := baseURL(cfg["url"]) + "/rpc.php"
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hr.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		hr.Header.Set("Cookie", cookie)
	}
	c := plainClient
	if truthy(cfg["insecure"]) {
		c = insecureClient
	}
	res, err := c.Do(hr)
	if err != nil {
		return "", explainNetErr("OpenMediaVault", endpoint, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("OpenMediaVault: %s not found (HTTP 404) — use the address of the OMV web UI", "/rpc.php")
	}
	var env struct {
		Response json.RawMessage `json:"response"`
		Error    *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("OpenMediaVault: unexpected reply (HTTP %d) — is this the OMV web UI address?", res.StatusCode)
	}
	if env.Error != nil {
		msg := strings.TrimSpace(env.Error.Message)
		if env.Error.Code == 5001 || strings.Contains(strings.ToLower(msg), "session") {
			return "", fmt.Errorf("OpenMediaVault: the session was refused (%s) — check the user's role", msg)
		}
		return "", fmt.Errorf("OpenMediaVault: %s", msg)
	}
	newCookie := ""
	for _, ck := range res.Cookies() {
		if strings.HasPrefix(ck.Name, "X-OPENMEDIAVAULT") || strings.HasPrefix(ck.Name, "PHPSESSID") {
			newCookie += ck.Name + "=" + ck.Value + "; "
		}
	}
	if out != nil && len(env.Response) > 0 && string(env.Response) != "null" {
		if err := json.Unmarshal(env.Response, out); err != nil {
			return "", fmt.Errorf("OpenMediaVault: unexpected data from %s.%s (%v)", service, method, err)
		}
	}
	return strings.TrimSuffix(newCookie, "; "), nil
}
