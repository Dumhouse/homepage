package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/go-chi/chi/v5"
)

// ---- projects ----

// projectCard is one card on the projects page (mockup: .pcard).
type projectCard struct {
	inventory.Project
	Envs       []envDot
	Down       int // checks not up across the project's services
	HostCount  int
	SvcCount   int
	RepoURL    string
	RepoKind   string // forgejo | github | git
	PublicURL  string
	OpenSource bool
}

type envDot struct {
	Name, Class string
}

type projectGroup struct {
	Name     string
	Projects []projectCard
}

type projectsPage struct {
	Layout
	Groups []projectGroup
	Total  int
	Filter string // "" | oss | issues
}

func (p *pages) projects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := p.shell(r, "Projects", "projects", crumbs(Crumb{Label: "Projects"}))
	l.Undo = undoFromQuery(r, "/projects/")
	filter := r.URL.Query().Get("f")
	all, err := p.inv.ListProjects(ctx)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	svcState := map[int64]string{}
	if list, err := p.chk.Store().List(ctx, "service", 0); err == nil {
		for _, c := range list {
			if c.Enabled && betterState(c.State, svcState[c.TargetID]) {
				svcState[c.TargetID] = c.State
			}
		}
	}
	groups := []projectGroup{}
	for _, proj := range all {
		full, err := p.inv.GetProject(ctx, proj.ID)
		if err != nil {
			continue
		}
		card := projectCard{Project: *full, OpenSource: full.OpenSource}
		hosts := map[int64]bool{}
		envWorst := map[string]string{}
		for _, h := range full.Hosts {
			hosts[h.HostID] = true
		}
		for _, v := range full.Services {
			hosts[v.HostID] = true
			st := svcState[v.ServiceID]
			if st != "" && st != "up" {
				card.Down++
			}
			if betterState(st, envWorst[v.Env]) {
				envWorst[v.Env] = st
			}
		}
		card.HostCount, card.SvcCount = len(hosts), len(full.Services)
		for _, e := range full.Envs {
			card.Envs = append(card.Envs, envDot{Name: e.Name, Class: map[string]string{"up": "ok", "failing": "warn", "down": "bad"}[envWorst[e.Name]]})
			if card.PublicURL == "" && e.BaseURL != "" {
				card.PublicURL = e.BaseURL
			}
		}
		for _, lk := range full.Links {
			switch {
			case lk.Kind == "repo" && card.RepoURL == "":
				card.RepoURL, card.RepoKind = lk.URL, "git"
				if strings.Contains(lk.URL, "github.com") {
					card.RepoKind = "github"
				}
			case lk.Kind == "public" && card.PublicURL == "":
				card.PublicURL = lk.URL
			}
		}
		if (filter == "oss" && !card.OpenSource) || (filter == "issues" && card.Down == 0) {
			continue
		}
		name := full.Group
		if name == "" {
			name = "Ungrouped"
		}
		if len(groups) == 0 || groups[len(groups)-1].Name != name {
			groups = append(groups, projectGroup{Name: name})
		}
		groups[len(groups)-1].Projects = append(groups[len(groups)-1].Projects, card)
	}
	p.ui.render(w, 200, "projects", projectsPage{Layout: l, Groups: groups, Total: len(all), Filter: filter})
}

type projectPage struct {
	Layout
	Project   *inventory.Project
	Env       *inventory.ProjectEnv
	EnvName   string
	AddEnv    bool
	EnvLinks  []inventory.AppLink
	LinkCheck map[int64]*checks.Check
	LinkBars  map[int64][]checks.Bar
	FormError string
	Access    map[int64]inventory.Access // access rows by id, for app-link login columns
	RepoURL   string
	SvcUp     map[int64]string // host id -> "n / m up"
}

func (p *pages) project(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	proj, err := p.inv.GetProjectBySlug(ctx, slug)
	if errors.Is(err, inventory.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil {
		p.fail(w, r, err)
		return
	}
	l := p.shell(r, proj.Name, "projects", crumbs(
		Crumb{Label: "Projects", Href: "/projects"},
		Crumb{Label: proj.Name},
	))
	l.Flash = flash(r)
	var env *inventory.ProjectEnv
	want := r.URL.Query().Get("env")
	selected := -1
	for i := range proj.Envs {
		if want != "" && proj.Envs[i].Name == want {
			selected = i
			break
		}
	}
	if selected == -1 && len(proj.Envs) > 0 {
		selected = 0
	}
	if selected >= 0 {
		env = &proj.Envs[selected]
	}
	envName := ""
	var links []inventory.AppLink
	if env != nil {
		envName = env.Name
		links = env.Links
	}
	linkChecks := map[int64]*checks.Check{}
	linkBars := map[int64][]checks.Bar{}
	for _, link := range proj.AppLinks {
		if list, err := p.chk.Store().List(ctx, "app_link", link.ID); err == nil && len(list) > 0 {
			linkChecks[link.ID] = &list[0]
			linkBars[link.ID], _ = p.chk.Store().Availability(ctx, list[0].ID, 12, 24)
		}
	}
	acc := map[int64]inventory.Access{}
	for _, a := range proj.Access {
		acc[a.ID] = a
	}
	repo := ""
	for _, lk := range proj.Links {
		if lk.Kind == "repo" && repo == "" {
			repo = lk.URL
		}
	}
	svcUp := map[int64]string{}
	if env != nil {
		for _, h := range env.Hosts {
			full, err := p.inv.GetHost(ctx, h.ID)
			if err != nil {
				continue
			}
			up, total := 0, 0
			for _, v := range full.Services {
				if list, err := p.chk.Store().List(ctx, "service", v.ID); err == nil && len(list) > 0 {
					total++
					if list[0].State == "up" {
						up++
					}
				}
			}
			if total > 0 {
				svcUp[h.ID] = strconv.Itoa(up) + " / " + strconv.Itoa(total) + " up"
			}
		}
	}
	p.ui.render(w, 200, "project", projectPage{
		Layout: l, Project: proj, Env: env, EnvName: envName, AddEnv: r.URL.Query().Get("addenv") != "", EnvLinks: links,
		LinkCheck: linkChecks, LinkBars: linkBars, FormError: formError(r),
		Access: acc, RepoURL: repo, SvcUp: svcUp,
	})
}
