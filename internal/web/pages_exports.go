package web

import (
	"encoding/json"
	"net/http"
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
