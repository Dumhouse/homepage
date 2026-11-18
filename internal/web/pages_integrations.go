package web

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Dumhouse/homepage/internal/changesets"
	"github.com/Dumhouse/homepage/internal/export"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/suggest"
	"github.com/go-chi/chi/v5"
)

// ---- integrations ----

type integrationsPage struct {
	Layout
	States     []integrations.State
	Suggested  []suggest.Suggestion
	Selected   string
	Auth       string
	Categories []string // sync plugin groups that have at least one plugin
	Exports    []export.Format
	BaseURL    string
}

func (p *pages) integrationsPage(w http.ResponseWriter, r *http.Request) {
	l := p.shell(r, "Integrations", "settings", crumbs(Crumb{Label: "Settings", Href: "/settings"}, Crumb{Label: "Integrations"}))
	l.Flash = flash(r)
	states := []integrations.State{}
	if p.intg != nil {
		var err error
		if states, err = p.intg.List(r.Context()); err != nil {
			p.fail(w, r, err)
			return
		}
	}
	configured := map[string]bool{}
	for _, st := range states {
		configured[st.ID] = st.Configured()
	}
	suggested, _ := suggest.For(r.Context(), p.inv, configured)
	page := integrationsPage{Layout: l, States: states, Suggested: suggested, Selected: r.URL.Query().Get("p"), Auth: os.Getenv("HS_AUTH")}
	if page.Auth == "" {
		page.Auth = "none"
	}
	page.Exports = export.Formats
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	page.BaseURL = scheme + "://" + r.Host
	used := map[string]bool{}
	for _, st := range states {
		if st.Plugin != nil && st.Plugin.Kind != integrations.KindNotify {
			used[st.Plugin.Category] = true
		}
	}
	for _, c := range append(append([]string{}, integrations.Categories...), "") {
		if used[c] {
			page.Categories = append(page.Categories, c)
		}
	}
	found := false
	for _, st := range states {
		found = found || st.ID == page.Selected
	}
	if !found && len(states) > 0 {
		page.Selected = states[0].ID
	}
	p.ui.render(w, 200, "integrations", page)
}

func (p *pages) integrationValues(r *http.Request, id string) map[string]string {
	values := map[string]string{}
	base, _, _ := strings.Cut(id, "~")
	for _, pl := range p.intg.Plugins() {
		if pl.ID != base {
			continue
		}
		for _, f := range pl.Fields {
			values[f.Key] = r.FormValue(f.Key)
		}
	}
	if r.Form.Has("_sync") {
		values["_sync"] = r.FormValue("_sync")
	}
	if r.Form.Has("_label") {
		values["_label"] = r.FormValue("_label")
	}
	return values
}

func (p *pages) integrationSave(w http.ResponseWriter, r *http.Request) {
	if p.intg == nil {
		p.notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id := chi.URLParam(r, "id")
	if err := p.intg.Save(r.Context(), actorOf(r), id, p.integrationValues(r, id)); err != nil {
		backWithError(w, r, "/settings/integrations?p="+url.QueryEscape(id), err)
		return
	}
	http.Redirect(w, r, "/settings/integrations?p="+url.QueryEscape(id)+"&ok=integration", http.StatusSeeOther)
}

func (p *pages) integrationTest(w http.ResponseWriter, r *http.Request) {
	if p.intg == nil {
		p.notFound(w, r)
		return
	}
	id := chi.URLParam(r, "id")
	back := "/settings/integrations?p=" + url.QueryEscape(id)
	// Test what is in the form, not only what was saved before.
	if err := r.ParseForm(); err == nil && len(r.PostForm) > 0 {
		if err := p.intg.Save(r.Context(), actorOf(r), id, p.integrationValues(r, id)); err != nil {
			backWithError(w, r, back, err)
			return
		}
	}
	if err := p.intg.Test(r.Context(), id); err != nil {
		backWithError(w, r, back, err)
		return
	}
	http.Redirect(w, r, back+"&ok=integration-tested", http.StatusSeeOther)
}

func (p *pages) integrationPropose(w http.ResponseWriter, r *http.Request) {
	if p.intg == nil {
		p.notFound(w, r)
		return
	}
	id := chi.URLParam(r, "id")
	states, err := p.intg.List(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	for _, st := range states {
		if st.ID != id {
			continue
		}
		ops := make([]changesets.Op, 0, len(st.Ops))
		for _, op := range st.Ops {
			ops = append(ops, changesets.Op{
				Action: op.Action, Entity: op.Entity, Ref: op.Ref, Data: op.Data, Note: op.Note,
			})
		}
		if _, err := p.cs.Create(r.Context(), actorOf(r), "Integration: "+st.Name, "plugin:"+st.ID, ops); err != nil {
			backWithError(w, r, "/settings/integrations", err)
			return
		}
		http.Redirect(w, r, "/changesets?ok=proposed", http.StatusSeeOther)
		return
	}
	p.notFound(w, r)
}

func (p *pages) integrationReset(w http.ResponseWriter, r *http.Request) {
	if p.intg == nil {
		p.notFound(w, r)
		return
	}
	id := chi.URLParam(r, "id")
	if err := p.intg.Reset(r.Context(), actorOf(r), id); err != nil {
		backWithError(w, r, "/settings/integrations?p="+url.QueryEscape(id), err)
		return
	}
	http.Redirect(w, r, "/settings/integrations?p="+url.QueryEscape(id)+"&ok=msg&msg="+url.QueryEscape("Disconnected — settings and tokens removed."), http.StatusSeeOther)
}

func (p *pages) integrationSync(w http.ResponseWriter, r *http.Request) {
	if p.intg == nil {
		p.notFound(w, r)
		return
	}
	id := chi.URLParam(r, "id")
	back := "/settings/integrations?p=" + url.QueryEscape(id)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, err := syncToChangeset(ctx, p.intg, p.cs, id, actorOf(r))
	if err != nil {
		backWithError(w, r, back, err)
		return
	}
	factsNote := ""
	if res.Facts > 0 {
		factsNote = "Updated facts of " + plural(res.Facts, "host") + ". "
	}
	if len(res.Missing) > 0 {
		factsNote += plural(len(res.Missing), "machine") + " not in the inventory yet (" + strings.Join(firstN(res.Missing, 5), ", ") + ") — their facts are applied with the new host. "
	}
	err = res.Warning
	if res.Changeset == nil {
		msg := factsNote + "Found " + plural(res.Found, "item") + "; nothing new to add to the inventory"
		if err != nil {
			msg += ". Note: " + err.Error()
		}
		http.Redirect(w, r, back+"&ok=msg&msg="+url.QueryEscape(msg), http.StatusSeeOther)
		return
	}
	c := res.Changeset
	target := "/changesets?ok=proposed#cs-" + itoa(c.ID)
	if err != nil || factsNote != "" {
		note := factsNote
		if err != nil {
			note += "Sync warning: " + err.Error()
		}
		target = "/changesets?ok=msg&msg=" + url.QueryEscape(strings.TrimSpace(note)+" New items are below for review.") + "#cs-" + itoa(c.ID)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func firstN(list []string, n int) []string {
	if len(list) > n {
		return append(list[:n:n], "…")
	}
	return list
}

// integrationAddInstance adds another copy of a plugin (a second Proxmox,
// another AdGuard…) and opens its settings.
func (p *pages) integrationAddInstance(w http.ResponseWriter, r *http.Request) {
	if p.intg == nil {
		p.notFound(w, r)
		return
	}
	_ = r.ParseForm()
	base, _, _ := strings.Cut(chi.URLParam(r, "id"), "~")
	id, err := p.intg.AddInstance(r.Context(), actorOf(r), base, r.FormValue("label"))
	if err != nil {
		backWithError(w, r, "/settings/integrations?p="+url.QueryEscape(base), err)
		return
	}
	http.Redirect(w, r, "/settings/integrations?p="+url.QueryEscape(id)+"&ok=msg&msg="+url.QueryEscape("Added — fill in its address and credentials below."), http.StatusSeeOther)
}
