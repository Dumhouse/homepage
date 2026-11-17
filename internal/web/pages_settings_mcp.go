package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/settings"
	"github.com/Dumhouse/homepage/internal/tokens"
)

// ---- settings · MCP ----

type mcpPage struct {
	Layout
	Tokens         []tokens.Token
	NewToken       string
	NewName        string
	FaviconFetch   string
	FaviconPrivate string
	Error          string
}

func (p *pages) settingsMCP(w http.ResponseWriter, r *http.Request) {
	l := p.shell(r, "MCP & tokens", "settings", crumbs(
		Crumb{Label: "Settings", Href: "/settings"},
		Crumb{Label: "MCP & tokens"},
	))
	l.Flash = flash(r)
	list, err := p.tok.List(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	p.ui.render(w, 200, "settings_mcp", mcpPage{
		Layout: l, Tokens: list,
		FaviconFetch:   settingOr(r, p.set, "favicon_fetch", "on"),
		FaviconPrivate: settingOr(r, p.set, "favicon_private", "off"),
	})
}

func settingOr(r *http.Request, s *settings.Store, key, def string) string {
	v, err := s.Get(r.Context(), key, def)
	if err != nil || v == "" {
		return def
	}
	return v
}

func (p *pages) saveFavicons(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	for key, value := range map[string]string{
		"favicon_fetch":   r.FormValue("favicon_fetch"),
		"favicon_private": r.FormValue("favicon_private"),
	} {
		if value != "on" && value != "off" {
			http.Error(w, "bad value", http.StatusBadRequest)
			return
		}
		if err := p.set.Set(r.Context(), actorOf(r), key, value); err != nil {
			p.fail(w, r, err)
			return
		}
	}
	http.Redirect(w, r, "/settings/favicons?ok=favicons", http.StatusSeeOther)
}

func (p *pages) createToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	scope := r.FormValue("scope")
	l := p.shell(r, "MCP & tokens", "settings", crumbs(
		Crumb{Label: "Settings", Href: "/settings"},
		Crumb{Label: "MCP & tokens"},
	))
	list, _ := p.tok.List(r.Context())
	t, err := p.tok.Create(r.Context(), name, scope)
	if err != nil {
		l.Flash = ""
		p.ui.render(w, 422, "settings_mcp", mcpPage{
			Layout: l, Tokens: list, NewName: name, Error: err.Error(),
			FaviconFetch:   settingOr(r, p.set, "favicon_fetch", "on"),
			FaviconPrivate: settingOr(r, p.set, "favicon_private", "off"),
		})
		return
	}
	l.Flash = "Token created — copy it now, it is shown only once."
	p.ui.render(w, 200, "settings_mcp", mcpPage{
		Layout: l, Tokens: list, NewToken: t.Plaintext, NewName: t.Name,
		FaviconFetch:   settingOr(r, p.set, "favicon_fetch", "on"),
		FaviconPrivate: settingOr(r, p.set, "favicon_private", "off"),
	})
}

func (p *pages) revokeToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := p.tok.Revoke(r.Context(), r.FormValue("name")); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/mcp?ok=revoked", http.StatusSeeOther)
}

type errorPage struct {
	Layout
	Code    int
	Message string
}

func (p *pages) notFound(w http.ResponseWriter, r *http.Request) {
	l := p.shell(r, "Not found", "error", crumbs(Crumb{Label: "Not found"}))
	p.ui.render(w, http.StatusNotFound, "error", errorPage{Layout: l, Code: 404, Message: "That page does not exist (or was deleted)."})
}

func (p *pages) fail(w http.ResponseWriter, r *http.Request, err error) {
	code := http.StatusInternalServerError
	msg := "Something went wrong."
	if errors.Is(err, inventory.ErrNotFound) {
		code, msg = http.StatusNotFound, "Not found."
	}
	if errors.Is(err, inventory.ErrInvalid) || errors.Is(err, settings.ErrInvalid) || errors.Is(err, tokens.ErrInvalid) {
		code, msg = http.StatusUnprocessableEntity, err.Error()
	}
	l := p.shell(r, "Error", "error", crumbs(Crumb{Label: "Error"}))
	p.ui.render(w, code, "error", errorPage{Layout: l, Code: code, Message: msg})
}

// savePrefs persists a UI preference from the sidebar toggle (fetch).
