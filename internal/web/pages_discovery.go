package web

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/dawidof/home-services/internal/discovery"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/go-chi/chi/v5"
)

// ---- discovery ----

type discoveryPage struct {
	Layout
	Hosts    []inventory.DiscoveredHost
	Subnet   string
	Subnets  []subnetCard
	LastScan string
	Show     string // new | ignored
	NewN     int
	IgnoredN int
	Status   discovery.Status
	Ports    string
	EveryH   string
}

type subnetCard struct {
	CIDR string
	New  int
	Seen int
	Size int
	Pct  int
}

func (p *pages) discoveryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := p.shell(r, "Discovery", "discovery", crumbs(Crumb{Label: "Discovery"}))
	show := "new"
	if r.URL.Query().Get("show") == "ignored" {
		show = "ignored"
	}
	fresh, err := p.inv.ListDiscovered(ctx, "new")
	if err != nil {
		p.fail(w, r, err)
		return
	}
	ignored, _ := p.inv.ListDiscovered(ctx, "ignored")
	all, _ := p.inv.ListDiscovered(ctx, "")
	nets := p.scan.Subnets(ctx)
	subnet := strings.Join(nets, ", ")
	last, _ := p.set.Get(ctx, discovery.KeyLastAt, "")
	every, _ := p.set.Get(ctx, discovery.KeyEveryH, "24")
	page := discoveryPage{Layout: l, Hosts: fresh, Subnet: subnet, LastScan: last, Show: show, NewN: len(fresh), IgnoredN: len(ignored),
		Status: p.scan.Status(), EveryH: every, Ports: joinInts(p.scan.Ports(ctx))}
	if show == "ignored" {
		page.Hosts = ignored
	}
	for _, cidr := range nets {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		ones, bits := n.Mask.Size()
		card := subnetCard{CIDR: cidr, Size: 1 << (bits - ones)}
		if card.Size > 2 {
			card.Size -= 2
		}
		for _, d := range all {
			if ip := net.ParseIP(d.Address); ip != nil && n.Contains(ip) {
				card.Seen++
				if d.Status == "new" {
					card.New++
				}
			}
		}
		if card.Size > 0 {
			card.Pct = card.Seen * 100 / card.Size
		}
		page.Subnets = append(page.Subnets, card)
	}
	l.Flash = flash(r)
	page.Layout = l
	p.ui.render(w, 200, "discovery", page)
}

func (p *pages) discoveryScan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	nets := p.scan.Subnets(r.Context())
	if only := discovery.ParseSubnets(r.FormValue("only")); len(only) > 0 {
		nets = only
	}
	if err := p.scan.Start(nets, actorOf(r)); err != nil && !errors.Is(err, discovery.ErrBusy) {
		backWithError(w, r, "/discovery", err)
		return
	}
	http.Redirect(w, r, "/discovery?ok=scan", http.StatusSeeOther)
}

func joinInts(list []int) string {
	parts := make([]string, len(list))
	for i, n := range list {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

func (p *pages) discoveryStatus(w http.ResponseWriter, r *http.Request) {
	reply(w, 200, p.scan.Status())
}

// discoverySubnets adds or removes one subnet, or replaces the list (settings form).
func (p *pages) discoverySubnets(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	back := safeBack(r.FormValue("back"), "/discovery")
	nets := p.scan.Subnets(ctx)
	switch {
	case r.Form.Has("list"):
		raw := r.FormValue("list")
		nets = discovery.ParseSubnets(raw)
		if strings.TrimSpace(raw) != "" && len(nets) == 0 {
			backWithError(w, r, back, fmt.Errorf("%w: no valid CIDR in %q", inventory.ErrInvalid, raw))
			return
		}
	case r.FormValue("add") != "":
		add := discovery.ParseSubnets(r.FormValue("add"))
		if len(add) == 0 {
			backWithError(w, r, back, fmt.Errorf("%w: %q is not a subnet like 192.168.1.0/24", inventory.ErrInvalid, r.FormValue("add")))
			return
		}
		nets = discovery.ParseSubnets(strings.Join(append(nets, add...), ","))
	case r.FormValue("remove") != "":
		kept := nets[:0]
		for _, n := range nets {
			if n != r.FormValue("remove") {
				kept = append(kept, n)
			}
		}
		nets = kept
	}
	if err := p.set.Set(ctx, actorOf(r), discovery.KeySubnets, strings.Join(nets, ",")); err != nil {
		p.fail(w, r, err)
		return
	}
	if r.Form.Has("ports") {
		raw := strings.TrimSpace(r.FormValue("ports"))
		ports := discovery.ParsePorts(raw)
		if raw != "" && len(ports) == 0 {
			backWithError(w, r, back, fmt.Errorf("%w: no valid port in %q — use numbers like 22, 80, 8000-8010", inventory.ErrInvalid, raw))
			return
		}
		_ = p.set.Set(ctx, actorOf(r), discovery.KeyPorts, joinInts(ports))
	}
	if r.Form.Has("every_h") {
		h, err := strconv.Atoi(strings.TrimSpace(r.FormValue("every_h")))
		if err != nil || h < 0 || h > 24*30 {
			backWithError(w, r, back, fmt.Errorf("%w: scan interval must be 0–720 hours", inventory.ErrInvalid))
			return
		}
		_ = p.set.Set(ctx, actorOf(r), discovery.KeyEveryH, strconv.Itoa(h))
	}
	if r.FormValue("scan") != "" && len(nets) > 0 {
		toScan := nets
		if added := discovery.ParseSubnets(r.FormValue("add")); len(added) > 0 {
			toScan = added // "add and scan" sweeps only the new subnet
		}
		_ = p.scan.Start(toScan, actorOf(r))
	}
	http.Redirect(w, r, withQuery(back, "ok=subnets"), http.StatusSeeOther)
}

func (p *pages) discoveryRegister(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	host, err := p.inv.RegisterDiscovered(r.Context(), actorOf(r), id, r.FormValue("name"))
	if err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/hosts/"+itoa(host.ID)+"?ok=created", http.StatusSeeOther)
}

func (p *pages) discoveryIgnore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.inv.IgnoreDiscovered(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/discovery", http.StatusSeeOther)
}
