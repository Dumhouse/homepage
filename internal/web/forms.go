package web

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Dumhouse/homepage/internal/inventory"
)

// ---- shared form bits ----

type addrSlot struct {
	N       int
	Kind    string
	Address string
}

type sshSlot struct {
	N     int
	User  string
	Port  int
	Vault string
}

func hostSlots(h *inventory.Host) ([]addrSlot, []sshSlot) {
	n := max(len(h.Addresses), 1)
	addrs := make([]addrSlot, n)
	for i := range addrs {
		addrs[i].N = i + 1
		if i < len(h.Addresses) {
			addrs[i].Kind = h.Addresses[i].Kind
			addrs[i].Address = h.Addresses[i].Address
		}
	}
	m := max(len(h.SSH), 1)
	ssh := make([]sshSlot, m)
	for i := range ssh {
		ssh[i].N = i + 1
		if i < len(h.SSH) {
			ssh[i].User = h.SSH[i].User
			ssh[i].Port = h.SSH[i].Port
			ssh[i].Vault = h.SSH[i].Vault
		}
	}
	return addrs, ssh
}

func intField(r *http.Request, name string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(r.FormValue(name)))
	return n
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func backWithError(w http.ResponseWriter, r *http.Request, path string, err error) {
	path = safeBack(path, "/")
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	http.Redirect(w, r, path+sep+"ok=err&msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
}

func formError(r *http.Request) string {
	if r.URL.Query().Get("ok") == "err" {
		return r.URL.Query().Get("msg")
	}
	return ""
}

// ---- host form ----

type hostFormPage struct {
	Layout
	IsNew        bool
	Action       string
	Host         *inventory.Host
	Locations    []inventory.Location
	Tags         []inventory.Tag
	LocationName string
	AddrSlots    []addrSlot
	SSHSlots     []sshSlot
	Error        string
}

func (p *pages) hostForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := int64(0)
	if v := chi.URLParam(r, "id"); v != "" && v != "new" {
		id, _ = strconv.ParseInt(v, 10, 64)
	}
	isNew := id == 0
	h := &inventory.Host{Kind: "vm", Lifecycle: "active"}
	locName := ""
	if !isNew {
		var err error
		h, err = p.inv.GetHost(ctx, id)
		if errors.Is(err, inventory.ErrNotFound) {
			p.notFound(w, r)
			return
		}
		if err != nil {
			p.fail(w, r, err)
			return
		}
		if h.Location != nil {
			locName = h.Location.Name
		}
	}
	locs, _ := p.inv.ListLocations(ctx)
	tags, _ := p.inv.ListTags(ctx)
	addrs, ssh := hostSlots(h)
	l := p.shell(r, "Host", "hosts", crumbs(
		Crumb{Label: "Hosts", Href: "/hosts"},
		Crumb{Label: h.Name},
	))
	title := "New host"
	if isNew {
		l.Crumbs = crumbs(Crumb{Label: "Hosts", Href: "/hosts"}, Crumb{Label: title})
	} else {
		l.Crumbs = crumbs(Crumb{Label: "Hosts", Href: "/hosts"}, Crumb{Label: h.Name, Href: "/hosts/" + itoa(id)}, Crumb{Label: "Edit"})
	}
	action := "/hosts"
	if !isNew {
		action = "/hosts/" + itoa(id)
	}
	p.ui.render(w, 200, "host_form", hostFormPage{
		Layout: l, IsNew: isNew, Action: action, Host: h, Locations: locs, Tags: tags,
		LocationName: locName, AddrSlots: addrs, SSHSlots: ssh,
	})
}

func (p *pages) hostSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := int64(0)
	if v := chi.URLParam(r, "id"); v != "" {
		id, _ = strconv.ParseInt(v, 10, 64)
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	h := inventory.Host{ID: id}
	h.Name = r.FormValue("name")
	h.Hostname = r.FormValue("hostname")
	h.Kind = r.FormValue("kind")
	h.Lifecycle = r.FormValue("lifecycle")
	h.OS = r.FormValue("os")
	h.Description = r.FormValue("description")
	h.Tags = splitTags(r.FormValue("tags"))
	for _, i := range rowIndexes(r, "addr_") {
		addr := strings.TrimSpace(r.FormValue("addr_" + itoa(int64(i))))
		if addr == "" {
			continue
		}
		kind := r.FormValue("addr_kind_" + itoa(int64(i)))
		h.Addresses = append(h.Addresses, inventory.Address{Kind: kind, Address: addr, IsPrimary: len(h.Addresses) == 0})
	}
	for _, i := range rowIndexes(r, "ssh_user_") {
		user := strings.TrimSpace(r.FormValue("ssh_user_" + itoa(int64(i))))
		if user == "" {
			continue
		}
		h.SSH = append(h.SSH, inventory.SSHAccess{
			User: user, Port: intField(r, "ssh_port_"+itoa(int64(i))), Vault: r.FormValue("ssh_vault_" + itoa(int64(i))),
		})
	}
	locName := strings.TrimSpace(r.FormValue("location"))
	loc, err := p.inv.EnsureLocation(ctx, actorOf(r), locName)
	if err == nil {
		h.LocationID = loc
	}
	if err == nil {
		if id == 0 {
			_, err = p.inv.CreateHost(ctx, actorOf(r), h)
		} else {
			_, err = p.inv.UpdateHost(ctx, actorOf(r), id, h)
		}
	}
	if err != nil {
		locs, _ := p.inv.ListLocations(ctx)
		tags, _ := p.inv.ListTags(ctx)
		addrs, ssh := hostSlots(&h)
		l := p.shell(r, "Host", "hosts", crumbs(Crumb{Label: "Hosts", Href: "/hosts"}, Crumb{Label: "Form"}))
		p.ui.render(w, 422, "host_form", hostFormPage{
			Layout: l, IsNew: id == 0, Action: r.URL.Path, Host: &h, Locations: locs, Tags: tags,
			LocationName: locName, AddrSlots: addrs, SSHSlots: ssh, Error: err.Error(),
		})
		return
	}
	if id == 0 {
		http.Redirect(w, r, "/hosts?ok=created", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/hosts/"+itoa(id)+"?ok=saved", http.StatusSeeOther)
}

func (p *pages) hostDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h, err := p.inv.GetHost(r.Context(), id)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := p.inv.DeleteHost(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/hosts?ok=deleted&id="+itoa(id)+"&name="+url.QueryEscape(h.Name), http.StatusSeeOther)
}

func (p *pages) hostRestore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.inv.RestoreHost(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/hosts?ok=restored", http.StatusSeeOther)
}

// ---- services (inline on the host page) ----

func (p *pages) serviceAdd(w http.ResponseWriter, r *http.Request) {
	hostID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	v := inventory.Service{
		Name: r.FormValue("name"), Type: r.FormValue("type"), Port: intField(r, "port"),
		Protocol: r.FormValue("protocol"), Path: r.FormValue("path"), Domain: r.FormValue("domain"),
	}
	created, err := p.inv.AddService(r.Context(), actorOf(r), hostID, v)
	if err == nil {
		p.icons.Queue(p.inv, created)
	}
	if err != nil {
		backWithError(w, r, "/hosts/"+itoa(hostID), err)
		return
	}
	http.Redirect(w, r, "/hosts/"+itoa(hostID)+"?ok=service", http.StatusSeeOther)
}

func (p *pages) serviceDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	svc, err := p.inv.GetService(r.Context(), id)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := p.inv.DeleteService(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/hosts/"+itoa(svc.HostID)+"?ok=service-deleted", http.StatusSeeOther)
}

// ---- project form ----

type projectFormPage struct {
	Layout
	IsNew   bool
	Action  string
	Project *inventory.Project
	Groups  []inventory.Group
	Error   string
}

func (p *pages) projectForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	isNew := slug == "" || slug == "new"
	proj := &inventory.Project{Status: "active"}
	if !isNew {
		var err error
		proj, err = p.inv.GetProjectBySlug(ctx, slug)
		if errors.Is(err, inventory.ErrNotFound) {
			p.notFound(w, r)
			return
		}
		if err != nil {
			p.fail(w, r, err)
			return
		}
	}
	groups, _ := p.inv.ListGroups(ctx)
	l := p.shell(r, "Project", "projects", crumbs(Crumb{Label: "Projects", Href: "/projects"}, Crumb{Label: "Form"}))
	action := "/projects"
	if isNew {
		l.Crumbs = crumbs(Crumb{Label: "Projects", Href: "/projects"}, Crumb{Label: "New project"})
	} else {
		l.Crumbs = crumbs(Crumb{Label: "Projects", Href: "/projects"}, Crumb{Label: proj.Name, Href: "/projects/" + proj.Slug}, Crumb{Label: "Edit"})
		action = "/projects/" + proj.Slug
	}
	p.ui.render(w, 200, "project_form", projectFormPage{
		Layout: l, IsNew: isNew, Action: action, Project: proj, Groups: groups,
	})
}

func (p *pages) projectSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	proj := inventory.Project{
		Name:        r.FormValue("name"),
		Slug:        r.FormValue("slug"),
		Description: r.FormValue("description"),
		Group:       r.FormValue("group"),
		Runbook:     r.FormValue("runbook_md"),
		Status:      r.FormValue("status"),
		OpenSource:  r.FormValue("is_open_source") != "",
	}
	var err error
	var saved *inventory.Project
	if slug == "" {
		saved, err = p.inv.CreateProject(ctx, actorOf(r), proj)
	} else {
		cur, gerr := p.inv.GetProjectBySlug(ctx, slug)
		if gerr != nil {
			p.fail(w, r, gerr)
			return
		}
		saved, err = p.inv.UpdateProject(ctx, actorOf(r), cur.ID, proj)
	}
	if err != nil {
		groups, _ := p.inv.ListGroups(ctx)
		l := p.shell(r, "Project", "projects", crumbs(Crumb{Label: "Projects", Href: "/projects"}, Crumb{Label: "Form"}))
		p.ui.render(w, 422, "project_form", projectFormPage{
			Layout: l, IsNew: slug == "", Action: r.URL.Path, Project: &proj, Groups: groups, Error: err.Error(),
		})
		return
	}
	target := "/projects?ok=created"
	if saved != nil && slug != "" {
		target = "/projects/" + saved.Slug + "?ok=saved"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (p *pages) projectDelete(w http.ResponseWriter, r *http.Request) {
	proj, err := p.inv.GetProjectBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := p.inv.DeleteProject(r.Context(), actorOf(r), proj.ID); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects?ok=deleted&kind=project&id="+itoa(proj.ID)+"&name="+url.QueryEscape(proj.Name), http.StatusSeeOther)
}

func (p *pages) projectRestore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.inv.RestoreProject(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects?ok=restored", http.StatusSeeOther)
}

// ---- project children ----

func (p *pages) envAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	proj, err := p.inv.GetProjectBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	e, err := p.inv.AddProjectEnv(ctx, actorOf(r), proj.ID, inventory.ProjectEnv{
		Name: r.FormValue("name"), BaseURL: r.FormValue("base_url"),
	})
	if err != nil {
		backWithError(w, r, "/projects/"+proj.Slug, err)
		return
	}
	http.Redirect(w, r, "/projects/"+proj.Slug+"?env="+url.QueryEscape(e.Name)+"&ok=env", http.StatusSeeOther)
}

func (p *pages) appLinkAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	proj, err := p.inv.GetProjectBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	env := r.FormValue("env")
	link := inventory.AppLink{
		Label: r.FormValue("label"), Path: r.FormValue("path"), OverrideURL: r.FormValue("url"),
		OnlyEnv: r.FormValue("only_env"), Kind: r.FormValue("kind"), HealthCheck: r.FormValue("health_check") != "",
	}
	if _, err := p.inv.AddAppLink(ctx, actorOf(r), proj.ID, link); err != nil {
		backWithError(w, r, projectBack(proj.Slug, env), err)
		return
	}
	target := projectBack(proj.Slug, env)
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	http.Redirect(w, r, target+sep+"ok=link", http.StatusSeeOther)
}

func projectBack(slug, env string) string {
	u := "/projects/" + slug
	if env != "" {
		u += "?env=" + url.QueryEscape(env)
	}
	return u
}

func (p *pages) appLinkDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.inv.DeleteAppLink(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back"), "/"), http.StatusSeeOther)
}

func (p *pages) accessDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	owner := "/"
	if a, err := p.inv.GetAccess(r.Context(), id); err == nil {
		owner, _ = p.accessTarget(r.Context(), a)
	}
	if err := p.inv.DeleteAccess(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, withQuery(safeBack(r.FormValue("back"), owner), "ok=access-deleted"), http.StatusSeeOther)
}

// safeBack only allows local paths, so a form cannot bounce users off-site.
// safeBack returns v only when it is a path on this app; anything that a browser could read as
// another site ("//evil", "/\\evil", "https://…", control characters) falls back to def.
func safeBack(v, def string) string {
	if !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || strings.ContainsAny(v, "\\\r\n\t") {
		return def
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return def
	}
	return v
}

func (p *pages) accessAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	a := inventory.Access{
		TargetEntity: r.FormValue("target_entity"), TargetID: int64(intField(r, "target_id")),
		Label: r.FormValue("label"), URL: r.FormValue("url"), Method: r.FormValue("method"),
		Account: r.FormValue("account"), TwoFA: r.FormValue("twofa"), VaultItemURL: r.FormValue("vault_item_url"),
		Notes: r.FormValue("notes"),
	}
	back := safeBack(r.FormValue("back"), "/hosts/"+itoa(a.TargetID))
	if _, err := p.inv.CreateAccess(ctx, actorOf(r), a); err != nil {
		backWithError(w, r, back, err)
		return
	}
	http.Redirect(w, r, back+"?ok=access", http.StatusSeeOther)
}

// rowIndexes returns the numeric suffixes of form fields named prefix+N, in
// order, so repeatable rows (added or removed in the browser) can be any count.
func rowIndexes(r *http.Request, prefix string) []int {
	_ = r.ParseForm()
	var out []int
	for k := range r.PostForm {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(k, prefix)); err == nil && n > 0 && n < 10000 {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}
