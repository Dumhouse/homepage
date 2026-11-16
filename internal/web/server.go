// Package web wires the HTTP router.
package web

import (
	"context"
	"database/sql"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	homeservices "github.com/dawidof/home-services"
	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/changesets"
	"github.com/dawidof/home-services/internal/checks"
	"github.com/dawidof/home-services/internal/discovery"
	"github.com/dawidof/home-services/internal/icons"
	"github.com/dawidof/home-services/internal/integrations"
	"github.com/dawidof/home-services/internal/inventory"
	hsmcp "github.com/dawidof/home-services/internal/mcp"
	"github.com/dawidof/home-services/internal/power"
	"github.com/dawidof/home-services/internal/request"
	"github.com/dawidof/home-services/internal/settings"
	"github.com/dawidof/home-services/internal/tokens"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is set by main from the release build (-X main.version).
var Version = "dev"

// Option customises the handler (tests keep the defaults).
type Option func(*options)

type options struct {
	checker        *checks.Scheduler
	dataDir        string
	fetcher        *icons.Fetcher
	intg           *integrations.Manager
	facts          FactsCollector
	scanner        *discovery.Runner
	power          *power.Store
	trustedProxies []*net.IPNet
}

// WithTrustedProxies trusts forwarding headers only from the given IPs/CIDRs
// (comma-separated). Without it, RemoteAddr is never rewritten.
func WithTrustedProxies(spec string) Option {
	nets := parseTrustedProxies(spec)
	return func(o *options) { o.trustedProxies = nets }
}

// WithPower shares the host power store (its poller is started by main).
func WithPower(s *power.Store) Option {
	return func(o *options) { o.power = s }
}

// WithScanner shares the background discovery runner (its schedule is started by main).
func WithScanner(r *discovery.Runner) Option {
	return func(o *options) { o.scanner = r }
}

// FactsCollector is the SSH fact collector (nil when HS_SSH_KEY is not set).
type FactsCollector interface {
	Collect(ctx context.Context, address string) (*inventory.HostFacts, error)
}

// WithFacts enables the "Collect SSH facts" action.
func WithFacts(f FactsCollector) Option {
	return func(o *options) { o.facts = f }
}

// WithIntegrations shares the registered integration plugins with API and UI.
func WithIntegrations(m *integrations.Manager) Option {
	return func(o *options) { o.intg = m }
}

// WithDataDir enables on-disk features (favicon cache) in that directory.
func WithDataDir(dir string) Option {
	return func(o *options) { o.dataDir = dir }
}

// WithChecker shares the running scheduler (with its alert notifier) with the
// UI and MCP, so on-demand checks emit the same alerts as the background loop.
func WithChecker(c *checks.Scheduler) Option {
	return func(o *options) { o.checker = c }
}

// New returns the application handler.
func New(db *sql.DB, a auth.Provider, opts ...Option) (http.Handler, error) {
	cfg := options{}
	for _, o := range opts {
		o(&cfg)
	}
	r := chi.NewRouter()
	r.Use(realIP(cfg.trustedProxies), middleware.Recoverer, middleware.Logger)
	r.Use(securityHeaders)
	// CSRF: browsers send Sec-Fetch-Site/Origin; a POST/PUT/PATCH/DELETE from another site is refused
	// for every route (forms, API, MCP). Non-browser clients (curl, agents) send neither and pass —
	// they are gated by tokens on /api/v1 and /mcp, and by UI auth on the forms.
	csrf := http.NewCrossOriginProtection()
	csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
	}))
	r.Use(csrf.Handler)

	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if err := db.PingContext(req.Context()); err != nil {
			http.Error(w, "db: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok " + Version + "\n"))
	})

	tok := tokens.New(db)
	inv := inventory.New(db)
	set := settings.New(db)
	chk := cfg.checker
	if chk == nil {
		chk = checks.NewScheduler(db, inv)
	}
	if cfg.dataDir != "" {
		cfg.fetcher = icons.New(filepath.Join(cfg.dataDir, "icons"), set)
	}
	cs := changesets.New(db, inv)
	pages, err := newPages(inv, tok, set, chk, cfg.fetcher, cfg.intg, cfg.facts, cs)
	if err != nil {
		return nil, err
	}
	pages.scan = cfg.scanner
	pages.pow = cfg.power
	if pages.pow == nil {
		pages.pow = power.NewStore(db)
	}
	if pages.scan == nil {
		pages.scan = discovery.NewRunner(inv, set)
	}
	cfg.scanner = pages.scan
	pages.routes(r)

	r.Group(func(r chi.Router) {
		r.Use(tokenAuth(tok, a, "api:", true))
		(&api{inv: inv, chk: chk, icons: cfg.fetcher, intg: cfg.intg, facts: cfg.facts, cs: cs, scan: cfg.scanner}).routes(r)
		r.Get("/inventory.md", func(w http.ResponseWriter, r *http.Request) {
			md, err := inv.InventoryMarkdown(r.Context())
			if err != nil {
				fail(w, err)
				return
			}
			writeMarkdown(w, md)
		})
	})
	// MCP: Streamable HTTP, stateless so any instance/request can serve a call.
	mcpServer := hsmcp.New(inv, chk, cs, cfg.intg)
	mcpReadOnly := hsmcp.NewReadOnly(inv, chk, cs, cfg.intg)
	mcpPropose := hsmcp.NewPropose(inv, chk, cs, cfg.intg)
	mcpHandler := sdkmcp.NewStreamableHTTPHandler(
		func(r *http.Request) *sdkmcp.Server {
			switch request.Scope(r.Context()) {
			case "write":
				return mcpServer
			case "propose":
				return mcpPropose // proposals only, no direct writes
			default:
				return mcpReadOnly
			}
		},
		&sdkmcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	r.Group(func(r chi.Router) {
		r.Use(tokenAuth(tok, a, "mcp:", false))
		r.Mount("/mcp", mcpHandler)
	})

	design, err := fs.Sub(homeservices.Design, "design")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(homeservices.UI, "static")
	if err != nil {
		return nil, err
	}
	assets, err := fs.Sub(design, "assets")
	if err != nil {
		return nil, err
	}
	if cfg.dataDir != "" {
		r.Handle("/icons/*", http.StripPrefix("/icons/", http.FileServer(http.Dir(filepath.Join(cfg.dataDir, "icons")))))
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(static)))
	// Shared prototype assets (app.css, boot.js) are still used by real pages.
	r.Handle("/assets/*", http.StripPrefix("/assets/", http.FileServerFS(assets)))
	// Screens that are not ported yet stay reachable under /design/ as a preview.
	r.Get("/design", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/design/dashboard.html", http.StatusFound)
	})
	r.Handle("/design/*", http.StripPrefix("/design/", http.FileServerFS(design)))
	return a.Wrap(r), nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		// themes can't load external CSS/scripts; icons may come from the LAN (favicon proxy comes later)
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: http: https:; script-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
