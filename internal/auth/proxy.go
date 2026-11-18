package auth

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/Dumhouse/homepage/internal/request"
)

// proxy trusts a user header set by a forward-auth proxy (Authelia, Authentik,
// Caddy forward_auth, oauth2-proxy, Tailscale serve…). The header is only
// believed when the direct peer is one of HS_TRUSTED_PROXIES; anyone else
// gets 403, so a client can never sign itself in by sending the header.
type proxy struct {
	header  string
	trusted []*net.IPNet
	allow   map[string]bool // empty = any user the proxy lets through
}

func newProxy(c Config) (Provider, error) {
	trusted := ParseNets(c.TrustedProxies)
	if len(trusted) == 0 {
		return nil, errors.New("HS_AUTH=proxy needs HS_TRUSTED_PROXIES (the address of your auth proxy)")
	}
	h := strings.TrimSpace(c.ProxyHeader)
	if h == "" {
		h = "Remote-User"
	}
	allow := map[string]bool{}
	for _, u := range strings.Split(c.ProxyUsers, ",") {
		if u = strings.ToLower(strings.TrimSpace(u)); u != "" {
			allow[u] = true
		}
	}
	return &proxy{header: h, trusted: trusted, allow: allow}, nil
}

func (p *proxy) Name() string { return "proxy" }

// user returns the signed-in user, or "" when the request did not come
// through the trusted proxy with an allowed user.
func (p *proxy) user(r *http.Request) string {
	if !peerIn(r.RemoteAddr, p.trusted) {
		return ""
	}
	u := strings.TrimSpace(r.Header.Get(p.header))
	if u == "" || strings.ContainsAny(u, "\r\n") || len(u) > 128 {
		return ""
	}
	if len(p.allow) > 0 && !p.allow[strings.ToLower(u)] {
		return ""
	}
	return u
}

func (p *proxy) ValidSession(r *http.Request) bool { return p.user(r) != "" }

func (p *proxy) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/healthz" || strings.HasPrefix(path, "/mcp") || strings.HasPrefix(path, "/api/v1") {
			next.ServeHTTP(w, r) // token-protected or public
			return
		}
		if path == "/logout" {
			http.Redirect(w, r, "/", http.StatusSeeOther) // sign-out happens at the proxy
			return
		}
		u := p.user(r)
		if u == "" {
			http.Error(w, "Forbidden: sign in through your authentication proxy.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(request.WithActor(r.Context(), "ui:"+u)))
	})
}

// ParseNets parses a comma-separated list of IPs and CIDRs. Invalid entries
// are skipped: a typo must never make every peer trusted.
func ParseNets(spec string) []*net.IPNet {
	out := []*net.IPNet{}
	for _, raw := range strings.Split(spec, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "/") {
			if _, n, err := net.ParseCIDR(raw); err == nil {
				out = append(out, n)
			}
			continue
		}
		ip := net.ParseIP(raw)
		if ip == nil {
			continue
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return out
}

func peerIn(remote string, nets []*net.IPNet) bool {
	host := remote
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
