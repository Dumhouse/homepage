package web

import (
	"net/http"
	"strings"

	"github.com/Dumhouse/homepage/internal/inventory"
)

type mapNode struct {
	inventory.GraphNode
	State string
	Deps  string // space-separated keys this node depends on (for the JS)
	Hit   bool   // affected by the focused node
	Focus bool
}

type mapColumn struct {
	Kind  string
	Title string
	Icon  string
	Nodes []mapNode
}

type mapPage struct {
	Layout
	Columns []mapColumn
	Focus   *mapNode
	Impact  map[string][]mapNode // kind → affected nodes
	Edges   int
	Linked  bool // only things that take part in a dependency
}

// dependencyMap shows projects → services → hosts → locations. With
// ?focus=h:3 it lists everything that breaks if that thing goes down.
func (p *pages) dependencyMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := p.shell(r, "Map", "map", crumbs(Crumb{Label: "Map"}))
	g, err := p.inv.DependencyGraph(ctx)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	states := map[string]string{}
	if list, err := p.chk.Store().List(ctx, "", 0); err == nil {
		for _, c := range list {
			k := ""
			switch c.TargetEntity {
			case "service":
				k = "s:" + itoa(c.TargetID)
			case "host":
				k = "h:" + itoa(c.TargetID)
			default:
				continue
			}
			if betterState(c.State, states[k]) {
				states[k] = c.State
			}
		}
	}
	deps := map[string][]string{}
	linked := map[string]bool{}
	for _, e := range g.Edges {
		deps[e.From] = append(deps[e.From], e.To)
		if !strings.HasPrefix(e.To, "l:") { // a host in a location alone is not "linked"
			linked[e.From], linked[e.To] = true, true
		}
	}
	focus := r.URL.Query().Get("focus")
	hit := map[string]bool{}
	if focus != "" {
		for _, k := range g.Impact(focus) {
			hit[k] = true
		}
	}
	onlyLinked := r.URL.Query().Get("all") == ""
	cols := []mapColumn{
		{Kind: "project", Title: "Projects", Icon: "folder"},
		{Kind: "service", Title: "Services", Icon: "activity"},
		{Kind: "host", Title: "Hosts", Icon: "server"},
		{Kind: "location", Title: "Locations", Icon: "globe"},
	}
	page := mapPage{Layout: l, Edges: len(g.Edges), Linked: onlyLinked, Impact: map[string][]mapNode{}}
	// Locations are shown when one of their hosts is shown.
	shownHost := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Kind == "host" && (!onlyLinked || linked[n.Key] || n.Key == focus) {
			shownHost[n.Key] = true
		}
	}
	shownLoc := map[string]bool{}
	for _, e := range g.Edges {
		if shownHost[e.From] && strings.HasPrefix(e.To, "l:") {
			shownLoc[e.To] = true
		}
	}
	for i := range cols {
		for _, n := range g.Nodes {
			if n.Kind != cols[i].Kind {
				continue
			}
			show := !onlyLinked || linked[n.Key] || n.Key == focus
			if n.Kind == "location" {
				show = !onlyLinked || shownLoc[n.Key] || n.Key == focus
			}
			if !show {
				continue
			}
			mn := mapNode{GraphNode: n, State: states[n.Key], Deps: strings.Join(deps[n.Key], " "),
				Hit: hit[n.Key], Focus: n.Key == focus}
			cols[i].Nodes = append(cols[i].Nodes, mn)
			if mn.Focus {
				f := mn
				page.Focus = &f
			}
			if mn.Hit {
				page.Impact[n.Kind] = append(page.Impact[n.Kind], mn)
			}
		}
	}
	page.Columns = cols
	p.ui.render(w, 200, "map", page)
}
