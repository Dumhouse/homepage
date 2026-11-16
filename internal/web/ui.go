package web

import (
	"bytes"
	"fmt"
	"github.com/dawidof/home-services/internal/discovery"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	homeservices "github.com/dawidof/home-services"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/yuin/goldmark"
)

// Layout is the shared shell data every page embeds.
type Layout struct {
	Title      string
	Page       string // active nav key
	Sidebar    string // open | collapsed
	Flash      string
	HostLabel  string
	Version    string
	Nav        []NavItem
	Ops        []NavItem
	Crumbs     []Crumb
	Stats      inventory.Stats
	TokenCount int
	Undo       *Undo
	Notices    []Notice // bell popover in the topbar
	Alerts     int      // problems + warnings counted on the bell
	Error      string   // ?ok=err message, shown as a toast unless the page shows it inline
}

// Notice is one row in the topbar bell popover.
type Notice struct {
	Icon  string
	Class string // bad | warn | info
	Title string
	Sub   string
	Href  string
	Btn   string
	Group string // "" = needs attention, "s" = suggestion
}

// Undo is the restore action shown in the delete toast.
type Undo struct {
	Path string
	Name string
}

// NavItem is one sidebar/topbar link.
type NavItem struct {
	Key        string
	Label      string
	Icon       string
	Href       string
	Count      int
	CountClass string
}

// Crumb is one breadcrumb step; Last is the current page (no link).
type Crumb struct {
	Label string
	Href  string
	Last  bool
}

func crumbs(items ...Crumb) []Crumb {
	for i := range items {
		items[i].Last = i == len(items)-1
	}
	return items
}

// ui holds one parsed template set per page, sharing layout.html.
type ui struct {
	pages map[string]*template.Template
	md    goldmark.Markdown
}

var pageFiles = map[string]string{
	"home":          "home.html",
	"hosts":         "hosts.html",
	"host":          "host.html",
	"host_form":     "host_form.html",
	"services":      "services.html",
	"locations":     "locations.html",
	"location_form": "location_form.html",
	"projects":      "projects.html",
	"project":       "project.html",
	"project_form":  "project_form.html",
	"settings_mcp":  "settings_mcp.html",
	"settings":      "settings.html",
	"problems":      "problems.html",
	"maintenance":   "maintenance.html",
	"domains":       "domains.html",
	"calendar":      "calendar.html",
	"costs":         "costs.html",
	"service_form":  "service_form.html",
	"discovery":     "discovery.html",
	"access_form":   "access_form.html",
	"integrations":  "integrations.html",
	"changesets":    "changesets.html",
	"app_link_form": "app_link_form.html",
	"domain":        "domain.html",
	"domain_form":   "domain_form.html",
	"error":         "error.html",
}

func newUI() (*ui, error) {
	u := &ui{pages: map[string]*template.Template{}, md: goldmark.New()}
	funcs := template.FuncMap{
		"icon":        icon,
		"markdown":    u.markdown,
		"fmtDate":     fmtDate,
		"day":         day,
		"inc":         func(n int) int { return n + 1 },
		"money":       money,
		"expiryBadge": expiryBadge,
		"priceValue":  priceValue,
		"truthy":      truthy,
		"kindIcon":    kindIcon,
		"stateClass":  stateClass,
		"hasPrefix":   func(s, p string) bool { return strings.HasPrefix(s, p) },
		"join":        strings.Join,
		"list":        func(v ...string) []string { return v },
		"plural":      plural,
		"colorFor":    colorFor,
		"initial":     initial,
		"upper":       strings.ToUpper,
		"title": func(s string) string {
			if s == "" {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
		"trimPrefix": strings.TrimPrefix,
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"serviceLink": func(h *inventory.Host, v inventory.Service) string { return serviceURL(h, &v) },
		"accessIcon": func(m string) string {
			switch m {
			case "ssh_key":
				return "terminal"
			case "token":
				return "key"
			case "none":
				return "unlock"
			}
			return "globe"
		},
		"linkIcon": func(kind string) string {
			return map[string]string{"page": "home", "admin": "edit", "jobs": "clock", "db": "db", "docs": "book",
				"health": "activity", "mcp": "bot", "other": "link"}[kind]
		},
		"linkColor": func(kind string) string {
			return map[string]string{"page": "#7c3aed", "admin": "#7c3aed", "jobs": "#b1003e", "db": "#336791", "docs": "#0ea5a4",
				"health": "#16a34a", "mcp": "#d97706", "other": "#64748b"}[kind]
		},
		"accessOf": func(m map[int64]inventory.Access, id *int64) *inventory.Access {
			if id == nil {
				return nil
			}
			if a, ok := m[*id]; ok {
				return &a
			}
			return nil
		},
		"hostOf": func(raw string) string {
			if u, err := url.Parse(raw); err == nil && u.Host != "" {
				return u.Host
			}
			return raw
		},
		"methodLabel": func(m string) string {
			return map[string]string{"password": "email + password", "google_sso": "Google sign-in", "github_sso": "GitHub sign-in",
				"oidc": "single sign-on", "ssh_key": "SSH key", "token": "token", "passkey": "passkey", "none": "no login"}[m]
		},
		"lcClass": func(lc string) string {
			if lc == "decommissioned" {
				return "decom"
			}
			return lc
		},
		"barClass": barClass,
		"portURL":  discovery.PortURL,
	}
	for name, file := range pageFiles {
		t, err := template.New("layout").Funcs(funcs).ParseFS(homeservices.UI,
			"templates/layout.html", "templates/pages/"+file)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		u.pages[name] = t
	}
	return u, nil
}

func (u *ui) render(w http.ResponseWriter, status int, page string, data any) {
	t, ok := u.pages[page]
	if !ok {
		http.Error(w, "unknown page", http.StatusInternalServerError)
		return
	}
	// Render into a buffer first so a template error becomes a real 500, not a half page with 200.
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("render %s: %v", page, err)
		http.Error(w, "page failed to render — see server log", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (u *ui) markdown(src string) template.HTML {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	var b bytes.Buffer
	if err := u.md.Convert([]byte(src), &b); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(b.String())
}

func writeMarkdown(w http.ResponseWriter, md string) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	io.WriteString(w, md)
}

// fmtDate turns SQLite's "2006-01-02 15:04:05" (UTC) into a short local label.
func fmtDate(s string) string {
	if s == "" {
		return ""
	}
	return when(s)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func money(cents int64, currency string) string {
	if cents == 0 {
		return "—"
	}
	if currency == "" {
		currency = "EUR"
	}
	return fmt.Sprintf("%.2f %s", float64(cents)/100, currency)
}

func truthy(s string) bool {
	return s == "1" || s == "true" || s == "on" || s == "yes"
}

func priceValue(cents int64) string {
	if cents == 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", float64(cents)/100)
}

func expiryBadge(days int) string {
	switch {
	case days < 0:
		return "bad"
	case days <= 30:
		return "warn"
	default:
		return "ok"
	}
}

func stateClass(state string) string {
	switch state {
	case "up":
		return "ok"
	case "failing", "recovering":
		return "warn"
	case "down":
		return "bad"
	default:
		return "outline"
	}
}

func kindIcon(kind string) string {
	switch kind {
	case "project":
		return "folder"
	case "service":
		return "activity"
	case "location":
		return "globe"
	case "setting":
		return "sliders"
	default:
		return "server"
	}
}

var iconPaths = map[string]string{
	"activity": `<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>`,
	"alert":    `<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>`,
	"arrow":    `<path d="M5 12h14M13 6l6 6-6 6"/>`,
	"arrowl":   `<path d="M19 12H5M11 6l-6 6 6 6"/>`,
	"calendar": `<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>`,
	"user":     `<circle cx="12" cy="8" r="4"/><path d="M4 21v-1a7 7 0 0 1 14 0v1"/>`,
	"bell":     `<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>`,
	"book":     `<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20V3H6.5A2.5 2.5 0 0 0 4 5.5z"/><path d="M4 19.5V21h16"/>`,
	"bot":      `<rect x="4" y="8" width="16" height="12" rx="3"/><path d="M12 4v4M9 13h.01M15 13h.01"/>`,
	"box":      `<path d="M21 8 12 3 3 8v8l9 5 9-5z"/><path d="m3 8 9 5 9-5M12 13v8"/>`,
	"check":    `<path d="M20 6 9 17l-5-5"/>`,
	"chevron":  `<path d="m6 9 6 6 6-6"/>`,
	"clock":    `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`,
	"cloud":    `<path d="M17.5 19H8a6 6 0 1 1 1.3-11.9A7 7 0 0 1 22 12a4.5 4.5 0 0 1-4.5 7z"/>`,
	"coin":     `<circle cx="12" cy="12" r="9"/><path d="M14.8 9.2A3 3 0 0 0 12.2 8h-.6a2.2 2.2 0 0 0 0 4.4h.8a2.2 2.2 0 0 1 0 4.4h-.7a3 3 0 0 1-2.6-1.2M12 6v2m0 8.8V18"/>`,
	"copy":     `<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>`,
	"cpu":      `<rect x="6" y="6" width="12" height="12" rx="2"/><path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4"/>`,
	"db":       `<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>`,
	"dots":     `<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`,
	"download": `<path d="M12 4v12M7 11l5 5 5-5M4 20h16"/>`,
	"edit":     `<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>`,
	"ext":      `<path d="M14 3h7v7M21 3l-9 9M19 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5"/>`,
	"external": `<path d="M14 3h7v7M21 3l-9 9M19 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h5"/>`,
	"eye":      `<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>`,
	"eyeoff":   `<path d="M3 3l18 18M10.6 5.1A10 10 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3 3.9M6.6 6.6C3.9 8.3 2 12 2 12s3.5 7 10 7c1.9 0 3.5-.6 4.9-1.4"/>`,
	"folder":   `<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`,
	"git":      `<circle cx="6" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="8" r="2.5"/><path d="M6 8.5v7M18 10.5c0 4-6 3-11 5.5"/>`,
	"globe":    `<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>`,
	"grid":     `<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>`,
	"home":     `<path d="M3 11 12 4l9 7"/><path d="M5 10v10h5v-6h4v6h5V10"/>`,
	"info":     `<circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h.01"/>`,
	"key":      `<circle cx="7.5" cy="15.5" r="4.5"/><path d="m10.7 12.3 9.3-9.3M17 6l3 3M15 8l2 2"/>`,
	"layout":   `<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M9 21V9"/>`,
	"link":     `<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/>`,
	"list":     `<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>`,
	"lock":     `<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>`,
	"merge":    `<circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="18" r="2.5"/><path d="M6 8.5V12a6 6 0 0 0 6 6h3.5"/>`,
	"moon":     `<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>`,
	"network":  `<rect x="9" y="2" width="6" height="6" rx="1"/><rect x="2" y="16" width="6" height="6" rx="1"/><rect x="16" y="16" width="6" height="6" rx="1"/><path d="M12 8v4M5 16v-2h14v2"/>`,
	"plus":     `<path d="M12 5v14M5 12h14"/>`,
	"power":    `<path d="M12 2v10M18.4 6.6a9 9 0 1 1-12.8 0"/>`,
	"radar":    `<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><path d="m12 12 6-6"/>`,
	"refresh":  `<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>`,
	"right":    `<path d="m9 6 6 6-6 6"/>`,
	"search":   `<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`,
	"server":   `<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>`,
	"shield":   `<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>`,
	"sidebar":  `<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>`,
	"sliders":  `<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>`,
	"star":     `<path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1 6.2L12 17.3 6.5 20.2l1-6.2L3 9.6l6.2-.9z"/>`,
	"sun":      `<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>`,
	"tag":      `<path d="M20 12.5 12.5 20a2 2 0 0 1-2.8 0L3 13.3V3h10.3L20 9.6a2 2 0 0 1 0 2.9z"/><path d="M7.5 7.5h.01"/>`,
	"terminal": `<path d="m4 17 6-6-6-6M12 19h8"/>`,
	"trash":    `<path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14"/>`,
	"unlock":   `<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.5-2"/>`,
	"upload":   `<path d="M12 16V4M7 9l5-5 5 5M4 20h16"/>`,
	"x":        `<path d="M18 6 6 18M6 6l12 12"/>`,
	"zap":      `<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>`,
}

func icon(name string, size int) template.HTML {
	p, ok := iconPaths[name]
	if !ok {
		p = iconPaths["info"]
	}
	return template.HTML(fmt.Sprintf(
		`<svg class="i" width="%d" height="%d" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">%s</svg>`,
		size, size, p))
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// barClass maps an availability bar to the mockup's .uptime classes ("" = up).
func barClass(c string) string {
	switch c {
	case "warn":
		return "w"
	case "bad":
		return "b"
	case "ok":
		return ""
	}
	return "n"
}
