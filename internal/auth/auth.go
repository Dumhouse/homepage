// Package auth provides pluggable UI authentication.
//
// Default is "none" (trusted LAN). Other providers are opt-in via HS_AUTH.
// MCP/API tokens are a separate mechanism and are not affected by this setting.
package auth

import (
	"fmt"
	"net/http"
	"strings"
)

// Provider decides who may use the web UI.
type Provider interface {
	// Name is the value of HS_AUTH that selects this provider.
	Name() string
	// Wrap returns a handler that enforces the provider's policy around next.
	// Routes that must stay open (health checks, login page) are handled by the provider itself.
	Wrap(next http.Handler) http.Handler
}

// SessionProvider is implemented by providers that can recognise a signed-in
// browser. The API token middleware uses it to let the UI through without a token.
type SessionProvider interface {
	ValidSession(r *http.Request) bool
}

// Config holds provider settings read from the environment.
type Config struct {
	Mode         string // none | password
	PasswordHash string // argon2id hash for "password"
	SessionKey   []byte // HMAC key for session cookies
	Secure       bool   // set Secure on cookies (true behind HTTPS)

	// proxy mode
	TrustedProxies string // HS_TRUSTED_PROXIES: only these peers may set the user header
	ProxyHeader    string // HS_AUTH_PROXY_HEADER, default Remote-User
	ProxyUsers     string // HS_AUTH_PROXY_USERS: optional comma-separated allow-list
}

// New builds the provider selected by cfg.Mode.
func New(cfg Config) (Provider, error) {
	switch strings.ToLower(cfg.Mode) {
	case "", "none":
		return None{}, nil
	case "password":
		return newPassword(cfg)
	case "proxy":
		return newProxy(cfg)
	default:
		return nil, fmt.Errorf("unknown auth mode %q (use none, password or proxy)", cfg.Mode)
	}
}

// None lets everyone in. Use only on a trusted network or behind your own access proxy.
type None struct{}

func (None) Name() string                        { return "none" }
func (None) Wrap(next http.Handler) http.Handler { return next }
func (None) ValidSession(*http.Request) bool     { return true }
