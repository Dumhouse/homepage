package integrations

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// req is one HTTP call of a read-only plugin.
type req struct {
	Method   string
	URL      string
	Header   map[string]string
	Body     io.Reader
	Insecure bool // self-signed certificates (BMCs, NAS boxes)
	What     string
}

var (
	plainClient    = &http.Client{Timeout: 15 * time.Second}
	insecureClient = &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in per integration
	}}
)

// getJSON performs r and decodes a JSON reply into out, turning the common
// failures into sentences that say what to fix.
func getJSON(ctx context.Context, r req, out any) error {
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	hr, err := http.NewRequestWithContext(ctx, r.Method, r.URL, r.Body)
	if err != nil {
		return err
	}
	hr.Header.Set("Accept", "application/json")
	for k, v := range r.Header {
		hr.Header.Set(k, v)
	}
	c := plainClient
	if r.Insecure {
		c = insecureClient
	}
	res, err := c.Do(hr)
	if err != nil {
		return explainNetErr(r.What, r.URL, err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%s refused the credentials (HTTP 401) — check the token or user", r.What)
	case res.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%s: the token has no permission for this (HTTP 403) — give it read access", r.What)
	case res.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%s: %s not found (HTTP 404) — check the URL", r.What, hr.URL.Path)
	case res.StatusCode >= 300:
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return fmt.Errorf("%s: HTTP %d %s", r.What, res.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: unexpected reply (%v) — is this the right URL?", r.What, err)
	}
	return nil
}

func explainNetErr(what, raw string, err error) error {
	host := raw
	if u, e := url.Parse(raw); e == nil {
		host = u.Host
	}
	var dns *net.DNSError
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return fmt.Errorf("%s: nothing listens on %s (connection refused) — check the port and that the service binds to the LAN, not only localhost", what, host)
	case errors.As(err, &dns):
		return fmt.Errorf("%s: can't resolve %s", what, dns.Name)
	case strings.Contains(err.Error(), "certificate"):
		return fmt.Errorf("%s: TLS certificate not trusted (%v) — tick “Accept self-signed certificate” if this is your own device", what, err)
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Timeout"):
		return fmt.Errorf("%s: %s didn't answer in time — firewall or wrong address?", what, host)
	}
	return fmt.Errorf("%s: %w", what, err)
}

// baseURL trims a configured URL and adds http:// when the scheme is missing.
func baseURL(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.Contains(s, "://") {
		s = "http://" + s
	}
	return strings.TrimRight(s, "/")
}

// urlHost is the host part (no port) of a URL, used as the device address.
func urlHost(raw string) string {
	u, err := url.Parse(baseURL(raw))
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// splitList splits a comma/space/newline separated list.
func splitList(s string) []string {
	out := []string{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' }) {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// humanUptime turns seconds into "12 days" / "5 h".
func humanUptime(sec float64) string {
	switch {
	case sec <= 0:
		return ""
	case sec >= 2*86400:
		return fmt.Sprintf("%d days", int(sec/86400))
	case sec >= 3600:
		return fmt.Sprintf("%d h", int(sec/3600))
	default:
		return fmt.Sprintf("%d min", int(sec/60))
	}
}

func pct(used, total float64) string {
	if total <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", int(used/total*100+0.5))
}

func gib(bytes float64) string { return fmt.Sprintf("%.0f", bytes/(1<<30)) }
