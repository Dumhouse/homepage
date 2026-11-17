package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/request"
	"github.com/Dumhouse/homepage/internal/tokens"
)

// actorUI is the audit actor for changes made through the web UI.
const actorUI = "ui"

func actorFrom(ctx context.Context) string {
	if a := request.Actor(ctx); a != "" {
		return a
	}
	return actorUI
}

// actorOf is the short form used by API handlers.
func actorOf(r *http.Request) string { return actorFrom(r.Context()) }

// tokenAuth gates /api/v1 and /mcp. A request passes when:
//   - it carries a valid UI session (password mode), or
//   - it is a browser request from the app itself with HS_AUTH=none, or
//   - it has a valid Bearer token (scope checked for API writes).
//
// The audit actor becomes ui, api:<token> or mcp:<token> and the scope is put
// in the context for MCP tools to enforce.
func tokenAuth(tok *tokens.Store, a auth.Provider, prefix string, checkScope bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sp, ok := a.(auth.SessionProvider); ok && sp.ValidSession(r) &&
				(a.Name() != "none" || sameOrigin(r)) {
				next.ServeHTTP(w, withIdentity(r, actorUI, "write"))
				return
			}
			if a.Name() == "none" && sameOrigin(r) {
				next.ServeHTTP(w, withIdentity(r, actorUI, "write"))
				return
			}
			raw := r.Header.Get("Authorization")
			if raw == "" {
				unauthorized(w)
				return
			}
			t, err := tok.Verify(r.Context(), raw)
			if err != nil {
				unauthorized(w)
				return
			}
			if checkScope && r.Method != http.MethodGet && r.Method != http.MethodHead &&
				r.Method != http.MethodOptions && !t.CanWrite() {
				// A propose token may only use the review queue.
				proposal := t.CanPropose() && r.Method == http.MethodPost && r.URL.Path == "/api/v1/changesets"
				if !proposal {
					reply(w, http.StatusForbidden, map[string]string{"error": "token scope cannot write"})
					return
				}
			}
			next.ServeHTTP(w, withIdentity(r, prefix+t.Name, t.Scope))
		})
	}
}

func withIdentity(r *http.Request, actor, scope string) *http.Request {
	ctx := request.WithActor(r.Context(), actor)
	ctx = request.WithScope(ctx, scope)
	return r.WithContext(ctx)
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="homeservices"`)
	reply(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
}

// sameOrigin detects the app's own browser requests (CSRF guard for HS_AUTH=none).
func sameOrigin(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "same-origin") {
		return true
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	return false
}
