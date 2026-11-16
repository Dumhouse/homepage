package web

import (
	"net"
	"net/http"
	"strings"
)

// parseTrustedProxies accepts a comma-separated list of IPs and CIDRs.
// Invalid entries are ignored: a typo must never make every peer trusted.
func parseTrustedProxies(spec string) []*net.IPNet {
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

// realIP replaces RemoteAddr with the client from a forwarding header, but only
// when the direct peer is one of the configured proxies. With no proxies set,
// headers are ignored — otherwise any client could spoof its address and evade
// the login rate limiter.
func realIP(trusted []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(trusted) > 0 && peerTrusted(r.RemoteAddr, trusted) {
				if ip := forwardedIP(r); ip != "" {
					r.RemoteAddr = ip
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func peerTrusted(remote string, trusted []*net.IPNet) bool {
	host := remote
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// forwardedIP prefers the header set by Cloudflare, then X-Real-IP, then the
// first valid address in X-Forwarded-For. Only call it for a trusted peer.
func forwardedIP(r *http.Request) string {
	for _, h := range []string{"CF-Connecting-IP", "X-Real-IP"} {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get(h))); ip != nil {
			return ip.String()
		}
	}
	for _, part := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if ip := net.ParseIP(strings.TrimSpace(part)); ip != nil {
			return ip.String()
		}
	}
	return ""
}
