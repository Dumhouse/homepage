# Design mockups

Static, clickable HTML mockups of Home Services — dark + light theme, desktop + mobile.
No build step; the CSS/JS here become `internal/web/static/` in the Go app.

```bash
python3 -m http.server 4173 --directory design   # then open http://localhost:4173
```

| Screen | File |
|---|---|
| Dashboard, Concept A · tiles (status, attention, numbers, tiles) | `dashboard.html` |
| Dashboard, Concept B · launcher (app icons, open-anything search, side panel) | `dashboard-launcher.html` |
| Hosts list (SSH key-only badges, filters, ssh-config export) | `hosts.html` |
| Host detail (services + health, SSH access, security, facts, timeline) | `host.html` |
| Projects (grouped cards with Forgejo/GitHub/public links) | `projects.html` |
| Project detail (app links per env, connected services, deploy pipeline, access, CI, runbook) | `project.html` |
| New / edit host form (comboboxes, repeaters, templates, icon picker) | `host-edit.html` |
| Discovery (subnet scans, inbox, drift, stale routes) | `discovery.html` |
| Network (IP map, Caddy routes, Proxmox storage) | `network.html` |
| Services & ports (all services, conflicts, free port) | `services.html` |
| Domains (providers, registrar/expiry/price, records matched to hosts) | `domains.html` |
| Domain detail (registration, records, account, cost history) | `domain.html` |
| Costs (recurring, hardware, by provider / project) | `costs.html` |
| Alerts (open, incident history with notes, maintenance windows) | `alerts.html` |
| Settings (integration catalog, secret store, MCP tokens, notifications, opening links, scanning) | `settings.html` |
| Topology map | `topology.html` |
| Decommission checklist (real case: openbao) | `decommission.html` |
| First-run setup: pick your tools (all optional) | `onboarding-tools.html` |
| First-run setup: review what was found | `onboarding.html` |

Global: ⌘K or `/` search · `N` Quick add (paste `ssh root@…`, `ip:port`, URL or `ct 118`) · `?` shortcuts ·
`g` + letter to jump · side panels with step-by-step fixes (e.g. *Fix SSH*) · `?` hover explanations ·
Undo on snooze/revoke/archive · copy-on-click · light/dark · mobile bottom nav.
Data is real where known (2026-11-16 scan); IPs/versions marked as examples are placeholders.
