package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Dumhouse/homepage/internal/export"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/go-chi/chi/v5"
)

// ---- exports ----

func (p *pages) exportSSH(w http.ResponseWriter, r *http.Request) {
	cfg, err := p.inv.SSHConfig(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.URL.Query().Get("inline") == "" {
		w.Header().Set("Content-Disposition", `attachment; filename="ssh-config"`)
	}
	w.Write([]byte(cfg))
}

func (p *pages) exportJSON(w http.ResponseWriter, r *http.Request) {
	data, err := p.inv.Export(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		p.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="homeservices-export.json"`)
	w.Write(b)
}

// exportNamed serves the generated files (Homepage, Ansible, Prometheus,
// NetBox). Reachable from the browser and, under /api/v1/export/, with an
// API token so other tools can fetch it on a schedule.
func (p *pages) exportNamed(w http.ResponseWriter, r *http.Request) {
	f, ok := export.Find(chi.URLParam(r, "name"))
	if !ok {
		p.notFound(w, r)
		return
	}
	list, err := p.inv.ListHosts(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	hosts := make([]inventory.Host, 0, len(list))
	for _, h := range list { // ListHosts leaves services out
		if full, err := p.inv.GetHost(r.Context(), h.ID); err == nil {
			hosts = append(hosts, *full)
		}
	}
	port, _ := strconv.Atoi(r.URL.Query().Get("port"))
	b, err := export.Render(f.Name, hosts, export.Options{Port: port, All: r.URL.Query().Get("all") != ""})
	if err != nil {
		p.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	if r.URL.Query().Get("download") != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+f.Name+`"`)
	}
	w.Write(b)
}
