package web

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dawidof/home-services/internal/checks"
	"github.com/dawidof/home-services/internal/inventory"
)

// ---- dashboard ----

type homeTile struct {
	Name       string
	Meta       string
	URL        string // what a click opens (site or details, per preference)
	SiteURL    string
	LocalURL   string
	PublicURL  string
	DetailsURL string
	Group      string
	Initial    string
	Color      string
	Kind       string
	Entity     string
	TargetID   int64
	Icon       string
	State      string // up | failing | down | "" (no check)
	Bars       []checks.Bar
	Extra      string // right side of the tile foot: latency or environments
	Off        bool   // planned / deprecated / decommissioned
	EditURL    string
	Tip        string // hover text: where it lives and what each check says
}

// DotClass is the status dot / launcher badge class.
func (t homeTile) DotClass() string {
	switch {
	case t.Off:
		return "off"
	case t.State == "down":
		return "bad"
	case t.State == "failing":
		return "warn"
	case t.State == "up":
		return "ok"
	}
	return ""
}

type tileGroup struct {
	Name  string
	Tiles []homeTile
}

// homeNum is one cell of the numbers strip.
type homeNum struct {
	Icon, Label, Href string
	Value, Of         string
	Meta, MetaClass   string
}

type homePage struct {
	Layout
	View        string
	TileClick   string
	Greeting    string
	StatusClass string // ok | warn | bad
	StatusWord  string
	StatusText  string
	Nums        []homeNum
	Tiles       []homeTile
	TileGroups  []tileGroup
	Groups      []string
	Group       string
	Problems    []checks.Problem
	Recent      []inventory.AuditEntry
	KeyOnly     int
	SSHTotal    int
	HostsUp     int
	NextExpiry  string
	AllCount    int
	ProblemN    int
}

func greeting(now time.Time) string {
	switch h := now.Hour(); {
	case h < 5:
		return "Good night"
	case h < 12:
		return "Good morning"
	case h < 18:
		return "Good afternoon"
	}
	return "Good evening"
}

func (p *pages) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	l := p.shell(r, "Dashboard", "dashboard", crumbs(Crumb{Label: "Dashboard"}))

	// View + tile-click preferences are remembered server-side.
	// They change through POST /prefs; a group filter link implies the tiles view without saving it.
	view, _ := p.set.Get(ctx, "home_view", "tiles")
	if q.Get("group") != "" {
		view = "tiles"
	}
	click, _ := p.set.Get(ctx, "tile_click", "details")

	hosts, err := p.inv.ListHosts(ctx)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	projects, err := p.inv.ListProjects(ctx)
	if err != nil {
		p.fail(w, r, err)
		return
	}

	// check state + latency per target
	type cstate struct {
		state   string
		latency int
	}
	states := map[string]cstate{}
	checkText := map[string][]string{} // "service:7" -> ["LAN up · 4 ms", "public down"]
	total, up := 0, 0
	if all, err := p.chk.Store().List(ctx, "", 0); err == nil {
		for _, c := range all {
			if !c.Enabled {
				continue
			}
			total++
			if c.State == "up" {
				up++
			}
			key := c.TargetEntity + ":" + itoa(c.TargetID)
			cur := states[key]
			if betterState(c.State, cur.state) {
				cur.state = c.State
			}
			if c.LastLatencyMS > 0 && (cur.latency == 0 || c.LastLatencyMS < cur.latency) {
				cur.latency = c.LastLatencyMS
			}
			states[key] = cur
			role := map[string]string{"lan": "LAN", "public": "Public"}[c.Role()]
			if role == "" {
				role = strings.ToUpper(c.Kind)
			}
			txt := role + " " + ternary(c.State == "", "not checked yet", c.State)
			if c.State == "up" && c.LastLatencyMS > 0 {
				txt += " · " + strconv.Itoa(c.LastLatencyMS) + " ms"
			} else if c.State != "up" && c.LastError != "" {
				txt += " · " + shortErr(c.LastError)
			}
			checkText[key] = append(checkText[key], txt)
		}
	}

	tiles := []homeTile{}
	keyOnly, sshTotal, hostsDown := 0, 0, 0
	for _, h := range hosts {
		full, err := p.inv.GetHost(ctx, h.ID)
		if err != nil {
			continue
		}
		off := full.Lifecycle != "" && full.Lifecycle != "active"
		addr := ""
		if len(full.Addresses) > 0 {
			addr = full.Addresses[0].Address
		}
		for _, s := range full.SSH {
			sshTotal++
			if s.Status == "key_only" {
				keyOnly++
			}
		}
		if st := states["host:"+itoa(h.ID)].state; st == "down" {
			hostsDown++
		}
		withURL := 0
		for _, v := range full.Services {
			site := serviceURL(full, &v)
			if site == "" {
				continue
			}
			withURL++
			local, public := "", ""
			if v.Port > 0 && addr != "" && (v.Protocol == "http" || v.Protocol == "https") {
				local = v.Protocol + "://" + addr + ":" + strconv.Itoa(v.Port) + v.Path
			}
			if v.Domain != "" {
				public = site
			}
			// mockup: the tile shows just where it is — the domain, else addr:port
			meta := v.Domain
			if meta == "" && addr != "" {
				meta = addr
				if v.Port > 0 {
					meta += ":" + strconv.Itoa(v.Port)
				}
			}
			tiles = append(tiles, homeTile{
				Name: v.Name, Meta: meta, SiteURL: site, LocalURL: local, PublicURL: public,
				DetailsURL: "/hosts/" + itoa(h.ID), EditURL: "/services/" + itoa(v.ID) + "/edit", Group: "Infrastructure",
				Initial: initial(v.Name), Color: colorFor(v.Name), Kind: "service",
				Entity: "service", TargetID: v.ID, Icon: v.Icon, Off: off, Tip: "on " + full.Name,
			})
		}
		if withURL == 0 { // a host with nothing to open still gets its own tile
			// A host has no check of its own: it counts as reachable when any of
			// its services or SSH logins answers.
			hs := cstate{}
			anyUp := false
			var parts []string
			for _, v := range full.Services {
				if c, ok := states["service:"+itoa(v.ID)]; ok && c.state != "" {
					anyUp = anyUp || c.state == "up"
					if betterState(c.state, hs.state) {
						hs.state = c.state
					}
					if c.latency > 0 && (hs.latency == 0 || c.latency < hs.latency) {
						hs.latency = c.latency
					}
					parts = append(parts, v.Name+" "+c.state)
				}
			}
			for _, x := range full.SSH {
				if c, ok := states["ssh_access:"+itoa(x.ID)]; ok && c.state != "" {
					anyUp = anyUp || c.state == "up"
					if betterState(c.state, hs.state) {
						hs.state = c.state
					}
					parts = append(parts, "SSH "+x.User+" "+c.state)
				}
			}
			if anyUp {
				hs.state = "up"
			}
			if _, own := states["host:"+itoa(h.ID)]; !own && hs.state != "" {
				states["host:"+itoa(h.ID)] = hs
				checkText["host:"+itoa(h.ID)] = append([]string{"reachable if anything on it answers"}, parts...)
			}
			tiles = append(tiles, homeTile{
				Name: full.Name, Meta: strings.TrimPrefix(addr+" · "+full.Kind, " · "),
				DetailsURL: "/hosts/" + itoa(h.ID), EditURL: "/hosts/" + itoa(h.ID) + "/edit", Group: "Infrastructure",
				Initial: initial(full.Name), Color: colorFor(full.Name), Kind: "host",
				Entity: "host", TargetID: h.ID, Off: off, Tip: full.Kind,
			})
		}
	}
	for _, proj := range projects {
		full, err := p.inv.GetProject(ctx, proj.ID)
		if err != nil {
			continue
		}
		group := full.Group
		if group == "" {
			group = "Projects"
		}
		site, meta := "", full.Slug
		if len(full.Envs) > 0 && full.Envs[0].BaseURL != "" {
			site = full.Envs[0].BaseURL
			meta = strings.TrimPrefix(strings.TrimPrefix(site, "https://"), "http://")
		}
		tiles = append(tiles, homeTile{
			Name: full.Name, Meta: meta, SiteURL: site, PublicURL: site, DetailsURL: "/projects/" + full.Slug,
			EditURL: "/projects/" + full.Slug + "/edit", Group: group,
			Initial: initial(full.Name), Color: colorFor(full.Name), Kind: "project",
			Entity: "project", TargetID: full.ID, Off: full.Status == "archived",
			Extra: strings.Join(envNames(full.Envs), " · "),
		})
	}

	groups := []string{}
	for _, t := range tiles {
		if !contains(groups, t.Group) {
			groups = append(groups, t.Group)
		}
	}
	groupRank := func(g string) string { // mockup order: Infrastructure, Apps, Projects, then the rest
		switch g {
		case "Infrastructure":
			return "0"
		case "Apps":
			return "1"
		case "Projects":
			return "2"
		}
		return "3" + strings.ToLower(g)
	}
	sort.Slice(groups, func(i, j int) bool { return groupRank(groups[i]) < groupRank(groups[j]) })
	allCount := len(tiles)
	filter := q.Get("group")
	if filter != "" {
		kept := tiles[:0]
		for _, t := range tiles {
			if t.Group == filter {
				kept = append(kept, t)
			}
		}
		tiles = kept
	}
	sort.SliceStable(tiles, func(i, j int) bool {
		if tiles[i].Group != tiles[j].Group {
			return groupRank(tiles[i].Group) < groupRank(tiles[j].Group)
		}
		return strings.ToLower(tiles[i].Name) < strings.ToLower(tiles[j].Name)
	})
	for i := range tiles {
		t := &tiles[i]
		cs := states[t.Entity+":"+itoa(t.TargetID)]
		t.State = cs.state
		if t.Entity == "service" {
			t.Bars, _ = p.chk.Store().AvailabilityN(ctx, t.Entity, t.TargetID, 40)
		}
		if t.Entity == "host" || t.Entity == "service" {
			if cs.latency > 0 {
				t.Extra = strconv.Itoa(cs.latency) + " ms"
			}
		}
		tip := []string{}
		if t.Tip != "" {
			tip = append(tip, t.Tip)
		}
		tip = append(tip, checkText[t.Entity+":"+itoa(t.TargetID)]...)
		if t.State == "" && !t.Off && t.Entity != "project" {
			tip = append(tip, "no check yet — give it a port or domain")
		}
		t.Tip = strings.Join(tip, " · ")
		t.URL = t.DetailsURL
		if click == "site" && t.SiteURL != "" {
			t.URL = t.SiteURL
		}
	}
	allProblems, _ := p.chk.Store().ListProblems(ctx, 200)
	problems := allProblems
	if len(problems) > 5 {
		problems = problems[:5]
	}
	for i := range problems {
		if problems[i].Title == "" {
			problems[i].Title = p.chk.Label(ctx, problems[i].TargetEntity, problems[i].TargetID)
		}
	}
	tileGroups := []tileGroup{}
	for _, t := range tiles {
		if len(tileGroups) == 0 || tileGroups[len(tileGroups)-1].Name != t.Group {
			tileGroups = append(tileGroups, tileGroup{Name: t.Group})
		}
		tileGroups[len(tileGroups)-1].Tiles = append(tileGroups[len(tileGroups)-1].Tiles, t)
	}

	// status line
	nProblems := len(allProblems)
	statusClass, statusWord := "ok", "All fine"
	switch {
	case total == 0:
		statusClass, statusWord = "off", "No checks yet"
	case nProblems > 0 && up*2 < total:
		statusClass, statusWord = "bad", "Several things are down"
	case nProblems > 0:
		statusClass, statusWord = "warn", "Mostly fine"
	}
	statusText := ""
	if total > 0 {
		statusText = " — " + strconv.Itoa(up) + " of " + plural(total, "check") + " up"
		if nProblems > 0 {
			statusText += ", " + plural(nProblems, "thing") + " need a look"
		}
	} else {
		statusText = " — add a service with a port and its check starts on its own"
	}

	// numbers strip (same five cells as the mockup)
	pwd := sshTotal - keyOnly
	nextDays, nextName := -1, ""
	if domains, err := p.inv.ListDomains(ctx); err == nil {
		today := time.Now().Truncate(24 * time.Hour)
		for _, d := range domains {
			t, err := time.Parse("2006-01-02", d.ExpiresAt)
			if err != nil || t.Before(today) {
				continue
			}
			days := int(t.Sub(today).Hours() / 24)
			if nextDays < 0 || days < nextDays {
				nextDays, nextName = days, d.Name
			}
		}
	}
	discovered, _ := p.inv.ListDiscovered(ctx, "new")
	svcDown := 0
	for _, pr := range allProblems {
		if pr.TargetEntity == "service" {
			svcDown++ // failing or down
		}
	}
	nums := []homeNum{
		{Icon: "server", Label: "Hosts", Href: "/hosts", Value: itoa(int64(len(hosts) - hostsDown)), Of: "/" + itoa(int64(len(hosts))),
			Meta: ternary(hostsDown > 0, plural(hostsDown, "host")+" down", "all reachable"), MetaClass: ternary(hostsDown > 0, "bad-t", "")},
		{Icon: "activity", Label: "Services", Href: "/hosts", Value: itoa(int64(l.Stats.Services - svcDown)), Of: "/" + itoa(int64(l.Stats.Services)),
			Meta: ternary(svcDown > 0, strconv.Itoa(svcDown)+" not up", "on "+plural(len(hosts), "host")), MetaClass: ternary(svcDown > 0, "bad-t", "")},
		{Icon: "key", Label: "SSH key-only", Href: "/hosts", Value: strconv.Itoa(keyOnly), Of: "/" + strconv.Itoa(sshTotal),
			Meta: ternary(pwd > 0, strconv.Itoa(pwd)+" not key-only yet", "all logins safe"), MetaClass: ternary(pwd > 0, "warn-t", "")},
		{Icon: "lock", Label: "Next expiry", Href: "/calendar", Value: ternary(nextDays >= 0, strconv.Itoa(nextDays), "—"), Of: ternary(nextDays >= 0, " days", ""),
			Meta: ternary(nextName != "", nextName, "nothing expiring"), MetaClass: ternary(nextDays >= 0 && nextDays <= 30, "warn-t", "")},
		{Icon: "radar", Label: "New on network", Href: "/discovery", Value: strconv.Itoa(len(discovered)),
			Meta: ternary(len(discovered) > 0, "not in the inventory", "nothing new")},
	}
	nextExpiry := "—"
	if nextDays >= 0 {
		nextExpiry = strconv.Itoa(nextDays) + " d"
	}

	recent, _ := p.inv.ListAudit(ctx, "", 0, 4)
	l.Flash = flash(r)
	p.ui.render(w, 200, "home", homePage{
		Layout: l, View: view, TileClick: click, Greeting: greeting(time.Now()),
		StatusClass: statusClass, StatusWord: statusWord, StatusText: statusText, Nums: nums,
		Tiles: tiles, TileGroups: tileGroups, Groups: groups, Group: filter, Problems: problems, Recent: recent,
		KeyOnly: keyOnly, SSHTotal: sshTotal, HostsUp: len(hosts) - hostsDown, NextExpiry: nextExpiry, AllCount: allCount, ProblemN: nProblems,
	})
}

func ternary(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

// saveHomePrefs stores the home view / tile-click choice. It is a POST so links, prefetchers and other
// sites can't change saved settings.
func (p *pages) saveHomePrefs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if v := r.FormValue("view"); v == "tiles" || v == "launcher" {
		_ = p.set.Set(ctx, actorOf(r), "home_view", v)
	}
	if v := r.FormValue("tile"); v == "details" || v == "site" {
		_ = p.set.Set(ctx, actorOf(r), "tile_click", v)
	}
	http.Redirect(w, r, safeBack(r.FormValue("back"), "/"), http.StatusSeeOther)
}

func betterState(next, prev string) bool {
	rank := func(s string) int {
		switch s {
		case "down":
			return 4
		case "failing":
			return 3
		case "recovering":
			return 2
		case "up":
			return 1
		default:
			return 0
		}
	}
	return rank(next) > rank(prev)
}

func expiringClass(n int) string {
	if n > 0 {
		return "warn"
	}
	return ""
}

func initial(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return "?"
}

var tileColors = []string{"#e57000", "#22b8cf", "#ff6600", "#317f6f", "#175ddc", "#d97706", "#16a34a", "#a855f7", "#e11d48", "#0891b2"}

func colorFor(s string) string {
	sum := 0
	for _, r := range s {
		sum += int(r)
	}
	return tileColors[sum%len(tileColors)]
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func envNames(envs []inventory.ProjectEnv) []string {
	out := make([]string, 0, len(envs))
	for _, e := range envs {
		out = append(out, e.Name)
	}
	return out
}

func serviceURL(h *inventory.Host, v *inventory.Service) string {
	if v.Domain != "" {
		scheme := "http"
		if v.Protocol == "https" {
			scheme = "https"
		}
		return scheme + "://" + v.Domain + v.Path
	}
	if v.Port > 0 && (v.Protocol == "http" || v.Protocol == "https") && len(h.Addresses) > 0 {
		return v.Protocol + "://" + h.Addresses[0].Address + ":" + strconv.Itoa(v.Port) + v.Path
	}
	return ""
}
