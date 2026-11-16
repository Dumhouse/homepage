package discovery

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed fingerprints.json
var fingerprintsJSON []byte

type fingerprint struct {
	Service string   `json:"service"`
	Title   []string `json:"title,omitempty"`
	Banner  []string `json:"banner,omitempty"`
	Ports   []int    `json:"ports,omitempty"`
}

var (
	fpOnce  sync.Once
	fpRules []fingerprint
)

func loadFingerprints() {
	fpOnce.Do(func() {
		_ = json.Unmarshal(fingerprintsJSON, &fpRules)
		for i := range fpRules {
			for j := range fpRules[i].Title {
				fpRules[i].Title[j] = strings.ToLower(fpRules[i].Title[j])
			}
			for j := range fpRules[i].Banner {
				fpRules[i].Banner[j] = strings.ToLower(fpRules[i].Banner[j])
			}
		}
	})
}

// fingerprintService matches a page title or TCP banner before the generic
// port table. Ports are only used for rules with no textual hint.
func fingerprintService(port int, title, banner string) string {
	loadFingerprints()
	hay := strings.ToLower(title + " \x00 " + banner)
	for _, r := range fpRules {
		for _, t := range r.Title {
			if t != "" && strings.Contains(hay, t) {
				return r.Service
			}
		}
		for _, b := range r.Banner {
			if b != "" && strings.Contains(hay, b) {
				return r.Service
			}
		}
		if len(r.Title) == 0 && len(r.Banner) == 0 {
			for _, p := range r.Ports {
				if p == port {
					return r.Service
				}
			}
		}
	}
	return ""
}
