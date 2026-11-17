package discovery

import (
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed fingerprints.json
var fingerprintsJSON []byte

type fingerprint struct {
	Service string   `json:"service"`
	Title   []string `json:"title,omitempty"`
	Banner  []string `json:"banner,omitempty"`
	Ports   []int    `json:"ports,omitempty"`
}

var (
	fpOnce  sync.Once
	fpRules []fingerprint
)

// probeRule identifies APIs that return JSON (or an auth error) on the root
// page instead of a title. Probing is bounded to one path per known port and
// only runs when the title/banner rules did not identify the service.
type probeRule struct {
	Service string
	Path    string
	Marker  string
}

var probePaths = map[int]probeRule{
	3000: {"grafana", "/api/health", `"database"`},
	8006: {"proxmox", "/api2/json/version", `"version"`},
	8080: {"ntfy", "/v1/health", `"healthy"`},
	8081: {"ntfy", "/v1/health", `"healthy"`},
	9000: {"portainer", "/api/status", "InstanceID"},
	9090: {"grafana", "/api/health", `"database"`},
	9443: {"portainer", "/api/status", "InstanceID"},
}

func loadFingerprints() {
	fpOnce.Do(func() {
		_ = json.Unmarshal(fingerprintsJSON, &fpRules)
		for i := range fpRules {
			for j := range fpRules[i].Title {
				fpRules[i].Title[j] = strings.ToLower(fpRules[i].Title[j])
			}
			for j := range fpRules[i].Banner {
				fpRules[i].Banner[j] = strings.ToLower(fpRules[i].Banner[j])
			}
		}
	})
}

// fingerprintService matches a page title or TCP banner before the generic
// port table. Ports are only used for rules with no textual hint.
func fingerprintService(port int, title, banner string) string {
	loadFingerprints()
	hay := strings.ToLower(title + " \x00 " + banner)
	for _, r := range fpRules {
		for _, t := range r.Title {
			if t != "" && strings.Contains(hay, t) {
				return r.Service
			}
		}
		for _, b := range r.Banner {
			if b != "" && strings.Contains(hay, b) {
				return r.Service
			}
		}
		if len(r.Title) == 0 && len(r.Banner) == 0 {
			for _, p := range r.Ports {
				if p == port {
					return r.Service
				}
			}
		}
	}
	return ""
}

// probeFingerprint tries the one probe path registered for a port.
func probeFingerprint(ctx context.Context, scheme, ip string, port int) string {
	rule, ok := probePaths[port]
	if !ok || scheme == "" {
		return ""
	}
	return probeAt(ctx, scheme+"://"+net.JoinHostPort(ip, strconv.Itoa(port)), rule)
}

// probeAt fetches rule.Path and matches rule.Marker. A 401/403 body is kept:
// several APIs answer an unauthenticated request with JSON explaining so.
func probeAt(ctx context.Context, baseURL string, rule probeRule) string {
	cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, strings.TrimRight(baseURL, "/")+rule.Path, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "homeservices-discovery/1")
	client := &http.Client{
		// #nosec G402 -- read-only fingerprinting must also see self-signed LAN endpoints.
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if strings.Contains(string(body), rule.Marker) {
		return rule.Service
	}
	return ""
}
