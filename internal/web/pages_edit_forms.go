package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/dawidof/home-services/internal/inventory"
	"github.com/go-chi/chi/v5"
)

// ---- edit forms ----

type serviceFormPage struct {
	Layout
	Service *inventory.Service
	Host    *inventory.Host
	Error   string
}

func (p *pages) serviceForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	svc, err := p.inv.GetService(ctx, id)
	if errors.Is(err, inventory.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil {
		p.fail(w, r, err)
		return
	}
	host, _ := p.inv.GetHost(ctx, svc.HostID)
	name := "service"
	if host != nil {
		name = host.Name
	}
	l := p.shell(r, svc.Name, "hosts", crumbs(
		Crumb{Label: "Hosts", Href: "/hosts"}, Crumb{Label: name, Href: "/hosts/" + itoa(svc.HostID)}, Crumb{Label: svc.Name},
	))
	p.ui.render(w, 200, "service_form", serviceFormPage{Layout: l, Service: svc, Host: host})
}

func (p *pages) serviceSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	svc := inventory.Service{
		Name: r.FormValue("name"), Type: r.FormValue("type"), Port: intField(r, "port"),
		Protocol: r.FormValue("protocol"), Path: r.FormValue("path"), Domain: r.FormValue("domain"),
		Version: r.FormValue("version"), Notes: r.FormValue("notes"), Tags: splitTags(r.FormValue("tags")),
	}
	got, err := p.inv.UpdateService(ctx, actorOf(r), id, svc)
	if err != nil {
		cur, _ := p.inv.GetService(ctx, id)
		hostID := int64(0)
		if cur != nil {
			hostID = cur.HostID
		}
		host, _ := p.inv.GetHost(ctx, hostID)
		l := p.shell(r, "Service", "hosts", crumbs(Crumb{Label: "Hosts", Href: "/hosts"}))
		p.ui.render(w, 422, "service_form", serviceFormPage{Layout: l, Service: cur, Host: host, Error: err.Error()})
		return
	}
	p.icons.Queue(p.inv, got)
	http.Redirect(w, r, "/hosts/"+itoa(got.HostID)+"?ok=saved", http.StatusSeeOther)
}

type accessFormPage struct {
	Layout
	Access *inventory.Access
	Back   string
	Target string
	Error  string
}

func (p *pages) accessForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	a, err := p.inv.GetAccess(ctx, id)
	if errors.Is(err, inventory.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil {
		p.fail(w, r, err)
		return
	}
	back, target := p.accessTarget(ctx, a)
	l := p.shell(r, a.Label, "hosts", crumbs(Crumb{Label: "Access"}, Crumb{Label: a.Label}))
	p.ui.render(w, 200, "access_form", accessFormPage{Layout: l, Access: a, Back: back, Target: target})
}

func (p *pages) accessTarget(ctx context.Context, a *inventory.Access) (string, string) {
	switch a.TargetEntity {
	case "host":
		if h, err := p.inv.GetHost(ctx, a.TargetID); err == nil {
			return "/hosts/" + itoa(h.ID), h.Name
		}
	case "service":
		if v, err := p.inv.GetService(ctx, a.TargetID); err == nil {
			return "/hosts/" + itoa(v.HostID), v.Name
		}
	case "project":
		if pr, err := p.inv.GetProject(ctx, a.TargetID); err == nil {
			return "/projects/" + pr.Slug, pr.Name
		}
	case "domain":
		if d, err := p.inv.GetDomain(ctx, a.TargetID); err == nil {
			return "/domains/" + itoa(d.ID), d.Name
		}
	}
	return "/", a.TargetEntity
}

func (p *pages) accessSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	a := inventory.Access{
		Label: r.FormValue("label"), URL: r.FormValue("url"), Method: r.FormValue("method"),
		Account: r.FormValue("account"), TwoFA: r.FormValue("twofa"),
		VaultItemURL: r.FormValue("vault_item_url"), Notes: r.FormValue("notes"),
	}
	if _, err := p.inv.UpdateAccess(ctx, actorOf(r), id, a); err != nil {
		cur, _ := p.inv.GetAccess(ctx, id)
		back, target := p.accessTarget(ctx, cur)
		l := p.shell(r, "Access", "hosts", crumbs(Crumb{Label: "Access"}))
		p.ui.render(w, 422, "access_form", accessFormPage{Layout: l, Access: cur, Back: back, Target: target, Error: err.Error()})
		return
	}
	cur, _ := p.inv.GetAccess(ctx, id)
	back, _ := p.accessTarget(ctx, cur)
	http.Redirect(w, r, back+"?ok=saved", http.StatusSeeOther)
}

type appLinkFormPage struct {
	Layout
	Link    *inventory.AppLink
	Project *inventory.Project
	Error   string
}

func (p *pages) appLinkForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	link, err := p.inv.GetAppLink(ctx, id)
	if errors.Is(err, inventory.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil {
		p.fail(w, r, err)
		return
	}
	proj, err := p.inv.GetProject(ctx, link.ProjectID)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	l := p.shell(r, link.Label, "projects", crumbs(
		Crumb{Label: "Projects", Href: "/projects"}, Crumb{Label: proj.Name, Href: "/projects/" + proj.Slug}, Crumb{Label: link.Label},
	))
	p.ui.render(w, 200, "app_link_form", appLinkFormPage{Layout: l, Link: link, Project: proj})
}

func (p *pages) appLinkSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	link := inventory.AppLink{
		Label: r.FormValue("label"), Path: r.FormValue("path"), OverrideURL: r.FormValue("url"),
		OnlyEnv: r.FormValue("only_env"), Kind: r.FormValue("kind"), Icon: r.FormValue("icon"),
		HealthCheck: r.FormValue("health_check") != "",
	}
	if v := intField(r, "access_id"); v > 0 {
		aid := int64(v)
		link.AccessID = &aid
	}
	if v := intField(r, "sort"); v != 0 {
		link.Sort = v
	}
	cur, _ := p.inv.GetAppLink(ctx, id)
	if _, err := p.inv.UpdateAppLink(ctx, actorOf(r), id, link); err != nil {
		proj, _ := p.inv.GetProject(ctx, cur.ProjectID)
		l := p.shell(r, "App link", "projects", crumbs(Crumb{Label: "Projects", Href: "/projects"}))
		p.ui.render(w, 422, "app_link_form", appLinkFormPage{Layout: l, Link: cur, Project: proj, Error: err.Error()})
		return
	}
	cur, _ = p.inv.GetAppLink(ctx, id)
	proj, _ := p.inv.GetProject(ctx, cur.ProjectID)
	http.Redirect(w, r, projectBack(proj.Slug, r.FormValue("env"))+"?ok=saved", http.StatusSeeOther)
}
