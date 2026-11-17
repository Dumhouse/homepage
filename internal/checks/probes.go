package checks

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Prober runs one check. Params are resolved by reconcile for auto checks and
// supplied directly for manual checks, so the prober never touches the inventory.
type Prober struct {
	Timeout time.Duration
}

func NewProber() *Prober { return &Prober{Timeout: 8 * time.Second} }

func (p *Prober) Run(ctx context.Context, c Check) Result {
	timeout := p.Timeout
	if sec := paramInt(c.Params, "timeout_s", 0); sec > 0 {
		timeout = time.Duration(sec) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch c.Kind {
	case "tcp":
		return p.tcp(ctx, c.Params)
	case "http":
		return p.http(ctx, c.Params)
	case "tls":
		return p.tlsProbe(ctx, c.Params)
	case "dns":
		return p.dns(ctx, c.Params)
	case "ssh":
		return p.sshProbe(ctx, c.Params)
	default:
		return Result{Message: "unknown check kind " + c.Kind}
	}
}

func paramString(p map[string]any, key, def string) string {
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func paramInt(p map[string]any, key string, def int) int {
	switch v := p[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

func paramBool(p map[string]any, key string) bool {
	switch v := p[key].(type) {
	case bool:
		return v
	case string:
		return v == "1" || v == "true"
	}
	return false
}

func (p *Prober) tcp(ctx context.Context, params map[string]any) Result {
	address := paramString(params, "address", "")
	port := paramInt(params, "port", 0)
	if address == "" || port == 0 {
		return Result{Message: "tcp check needs address and port"}
	}
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{LatencyMS: ms, Message: fmt.Sprintf("connect %s:%d: %v", address, port, err)}
	}
	conn.Close()
	return Result{OK: true, LatencyMS: ms, Message: fmt.Sprintf("connected in %d ms", ms)}
}

func (p *Prober) http(ctx context.Context, params map[string]any) Result {
	url := paramString(params, "url", "")
	if url == "" {
		return Result{Message: "http check needs url"}
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: paramBool(params, "ignore_tls")}}
	client := &http.Client{Transport: tr}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{Message: err.Error()}
	}
	req.Header.Set("User-Agent", "homeservices-check/1")
	start := time.Now()
	resp, err := client.Do(req)
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{LatencyMS: ms, Message: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	ok, want := statusInRange(resp.StatusCode, paramString(params, "expect_status", "200-399"))
	if !ok {
		return Result{LatencyMS: ms, Message: fmt.Sprintf("HTTP %d (expected %s)", resp.StatusCode, want)}
	}
	if kw := paramString(params, "keyword", ""); kw != "" && !strings.Contains(string(body), kw) {
		return Result{LatencyMS: ms, Message: fmt.Sprintf("HTTP %d but keyword %q not found", resp.StatusCode, kw)}
	}
	return Result{OK: true, LatencyMS: ms, Message: fmt.Sprintf("HTTP %d in %d ms", resp.StatusCode, ms)}
}

func statusInRange(code int, spec string) (bool, string) {
	if spec == "" {
		spec = "200-399"
	}
	if i := strings.IndexByte(spec, '-'); i > 0 {
		lo, _ := strconv.Atoi(strings.TrimSpace(spec[:i]))
		hi, _ := strconv.Atoi(strings.TrimSpace(spec[i+1:]))
		if lo == 0 && hi == 0 {
			return true, spec
		}
		return code >= lo && code <= hi, spec
	}
	if spec == "any" {
		return true, spec
	}
	want, _ := strconv.Atoi(spec)
	return code == want, spec
}

func (p *Prober) tlsProbe(ctx context.Context, params map[string]any) Result {
	address := paramString(params, "address", "")
	serverName := paramString(params, "server_name", "")
	if address == "" {
		return Result{Message: "tls check needs address (host:port)"}
	}
	if !strings.Contains(address, ":") {
		address = net.JoinHostPort(address, "443")
	}
	warnDays := paramInt(params, "warn_days", 14)
	start := time.Now()
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{LatencyMS: ms, Message: err.Error()}
	}
	defer raw.Close()
	conn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, ServerName: serverName})
	if err := conn.HandshakeContext(ctx); err != nil {
		return Result{LatencyMS: ms, Message: "tls handshake: " + err.Error()}
	}
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return Result{LatencyMS: ms, Message: "server sent no certificate"}
	}
	cert := state.PeerCertificates[0]
	days := int(time.Until(cert.NotAfter).Hours() / 24)
	if serverName != "" {
		if err := cert.VerifyHostname(serverName); err != nil {
			return Result{LatencyMS: ms, Message: "certificate does not match " + serverName}
		}
	}
	issuer := cert.Issuer.Organization
	name := issuer[0]
	if name == "" {
		name = cert.Issuer.CommonName
	}
	if days < warnDays {
		return Result{LatencyMS: ms, Message: fmt.Sprintf("expires in %d days (%s)", days, name),
			CertDays: days, CertIssuer: name}
	}
	return Result{OK: true, LatencyMS: ms, Message: fmt.Sprintf("expires in %d days (%s)", days, name),
		CertDays: days, CertIssuer: name}
}

func (p *Prober) dns(ctx context.Context, params map[string]any) Result {
	name := paramString(params, "name", "")
	if name == "" {
		return Result{Message: "dns check needs name"}
	}
	start := time.Now()
	ips, err := net.DefaultResolver.LookupHost(ctx, name)
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{LatencyMS: ms, Message: err.Error()}
	}
	expect := paramString(params, "expect_ip", "")
	if expect != "" {
		for _, ip := range ips {
			if ip == expect {
				return Result{OK: true, LatencyMS: ms, Message: name + " → " + expect}
			}
		}
		return Result{LatencyMS: ms, Message: fmt.Sprintf("%s resolves to %s, expected %s", name, strings.Join(ips, ", "), expect)}
	}
	return Result{OK: true, LatencyMS: ms, Message: name + " → " + strings.Join(ips, ", ")}
}

func (p *Prober) sshProbe(ctx context.Context, params map[string]any) Result {
	address := paramString(params, "address", "")
	port := paramInt(params, "port", 22)
	if address == "" {
		return Result{Message: "ssh check needs address"}
	}
	addr := net.JoinHostPort(address, strconv.Itoa(port))
	start := time.Now()
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{LatencyMS: ms, Message: "unreachable: " + err.Error(), SSHStatus: "unreachable"}
	}
	defer raw.Close()
	cfg := &ssh.ClientConfig{
		User:            paramString(params, "user", "homeservices-probe"),
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         p.Timeout,
	}
	_, _, _, err = ssh.NewClientConn(raw, addr, cfg)
	if err == nil {
		// A server that accepts an empty password: treat it as password auth.
		return Result{OK: true, LatencyMS: ms, Message: "ssh accepts passwords", SSHStatus: "password_allowed"}
	}
	msg := err.Error()
	if strings.Contains(msg, "password") || strings.Contains(msg, "keyboard-interactive") {
		return Result{OK: true, LatencyMS: ms, Message: "ssh allows password auth — switch to keys", SSHStatus: "password_allowed"}
	}
	if strings.Contains(msg, "unable to authenticate") || strings.Contains(msg, "no supported methods") {
		return Result{OK: true, LatencyMS: ms, Message: "ssh is key-only", SSHStatus: "key_only"}
	}
	return Result{LatencyMS: ms, Message: "ssh: " + msg, SSHStatus: "unreachable"}
}
