package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/dawidof/home-services/internal/inventory"
)

type locationsPage struct {
	Layout
	Locations []inventory.Location
}

func (p *pages) locations(w http.ResponseWriter, r *http.Request) {
	l := p.shell(r, "Locations", "locations", crumbs(Crumb{Label: "Locations"}))
	l.Undo = undoFromQuery(r, "/locations/")
	list, err := p.inv.ListLocations(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	l.Flash = flash(r)
	p.ui.render(w, 200, "locations", locationsPage{Layout: l, Locations: list})
}

type locationFormPage struct {
	Layout
	IsNew    bool
	Action   string
	Location *inventory.Location
	Error    string
	Kinds    []string
}

var locationKinds = []string{"proxmox", "vps", "dedicated", "cloud", "bare-metal", "other"}

func (p *pages) locationForm(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	isNew := id == 0
	loc := &inventory.Location{Kind: "other"}
	if !isNew {
		var err error
		loc, err = p.inv.GetLocation(r.Context(), id)
		if errors.Is(err, inventory.ErrNotFound) {
			p.notFound(w, r)
			return
		}
		if err != nil {
			p.fail(w, r, err)
			return
		}
	}
	l := p.shell(r, "Location", "locations", crumbs(Crumb{Label: "Locations", Href: "/locations"}, Crumb{Label: "Form"}))
	action := "/locations"
	if isNew {
		l.Crumbs = crumbs(Crumb{Label: "Locations", Href: "/locations"}, Crumb{Label: "New location"})
	} else {
		l.Crumbs = crumbs(Crumb{Label: "Locations", Href: "/locations"}, Crumb{Label: loc.Name})
		action = "/locations/" + itoa(id)
	}
	p.ui.render(w, 200, "location_form", locationFormPage{Layout: l, IsNew: isNew, Action: action, Location: loc, Kinds: locationKinds})
}

func (p *pages) locationSave(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	loc := inventory.Location{
		Name: r.FormValue("name"), Kind: r.FormValue("kind"), URL: r.FormValue("url"), Notes: r.FormValue("notes"),
	}
	var err error
	if id == 0 {
		_, err = p.inv.CreateLocation(r.Context(), actorOf(r), loc)
	} else {
		_, err = p.inv.UpdateLocation(r.Context(), actorOf(r), id, loc)
	}
	if err != nil {
		l := p.shell(r, "Location", "locations", crumbs(Crumb{Label: "Locations", Href: "/locations"}, Crumb{Label: "Form"}))
		p.ui.render(w, 422, "location_form", locationFormPage{Layout: l, IsNew: id == 0, Action: r.URL.Path, Location: &loc, Error: err.Error(), Kinds: locationKinds})
		return
	}
	if id == 0 {
		http.Redirect(w, r, "/locations?ok=created", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/locations?ok=saved", http.StatusSeeOther)
}

func (p *pages) locationDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	loc, err := p.inv.GetLocation(r.Context(), id)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := p.inv.DeleteLocation(r.Context(), actorOf(r), id); err != nil {
		backWithError(w, r, "/locations", err)
		return
	}
	http.Redirect(w, r, "/locations?ok=deleted&id="+itoa(id)+"&name="+url.QueryEscape(loc.Name), http.StatusSeeOther)
}

func (p *pages) locationRestore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.inv.RestoreLocation(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/locations?ok=restored", http.StatusSeeOther)
}
