# Home Services — Plan

A lightweight, self-hosted inventory + monitoring app for LAN/remote projects:
hosts, SSH access, services, projects, links, health, discovery, alerts —
with a dashboard mode and an MCP server so AI agents can read/write records.

---

## 1. Stack

| Layer | Choice | Why |
|---|---|---|
| Language | **Go** (1.23+) | single static binary, ~20 MB RAM idle, native SSH + fast concurrent probes |
| DB | **SQLite** (WAL, `modernc.org/sqlite` – pure Go, no CGO) | one file, trivial backup |
| Migrations | `pressly/goose` (embedded SQL) | simple, versioned |
| Queries | hand-written SQL in `internal/inventory` (revisit sqlc past ~2k lines) | store is small; no codegen step |
| HTTP | `net/http` + `chi` router | minimal |
| UI | server-rendered `templ` + **htmx** + Alpine.js (small bits) + Pico/own CSS | no JS build step, assets embedded in binary |
| SSH | `golang.org/x/crypto/ssh` | auth-method probing + fact collection |
| MCP | `github.com/modelcontextprotocol/go-sdk` | official SDK, Streamable HTTP transport |
| Jobs | in-process scheduler (goroutines + ticker, jobs table) | no Redis/queue needed |
| Alerts | ntfy HTTP API | already running in an ntfy LXC |

Everything (templates, CSS, JS, icons, migrations) is embedded → deploy = copy one binary.

---

## 2. Example environment

The project was designed against a typical Proxmox home lab. The maintainer's real addresses and
inventory live in a private deployment repository; public examples use `192.168.1.0/24` (LAN) and
`192.168.2.0/24` (a second, routed subnet).

| Thing | Example |
|---|---|
| Hypervisor | Proxmox VE, node `192.168.1.250` |
| LAN | `192.168.1.0/24`, gateway `.1` |
| Second subnet | `192.168.2.0/24` behind a router, reached over a static route |
| This app | an unprivileged Debian LXC at `192.168.1.101:8080` |
| Reverse proxy | Caddy with its admin API on `127.0.0.1:2019` |
| Notifications | any ntfy topic |
| Secrets | Vaultwarden links for humans, an external manager for machine tokens |
| DNS | Cloudflare, OVH or manual records |

---

## 3. Data model

```
Location ──< Host ──< HostAddress        (lan / tailscale / wireguard / public)
              │  ──< SshAccess           (user, port, key-only status, last check)
              │  ──< Service ──< Check ──< CheckResult
              │  ──< HostFact            (os, uptime, disk, updates, reboot, units…)
              │  ──< AuthorizedKey       (fingerprint, comment, user)
              │  ──< Snippet             (saved commands)
              │  ──< TimelineEntry
Project ──< ProjectLink                  (forgejo / github / public / docs / other)
   │   ──< ProjectHost / ProjectService  (per Environment: prod/staging/dev)
   │   >── Group
   └── Tags (many-to-many, shared with Host/Service)
Subnet ──< (Host addresses), VLAN
DiscoveredHost / DiscoveredPort         (inbox of unregistered findings)
Domain ──< DnsRecord                    (cloudflare / ovh), TLS cert info
Backup                                  (target, schedule, last_success, last_verified)
AlertRule / Notification / MaintenanceWindow
AuditLog                                (actor: user | mcp:<token-name>, diff)
ApiToken                                (name, scope: read | write, hashed)
Setting                                 (key/value)
```

### Key tables (columns abridged)

- **locations**: id, name (`Proxmox .250`, `OVH`, …), kind (proxmox/vps/dedicated/cloud/bare-metal), url, notes, monthly_cost
- **hosts**: id, location_id, name, hostname, kind (vm/lxc/bare/vps), proxmox_vmid, os, cpu, ram_mb, disk_gb, mac, icon, description, monthly_cost, tags, deleted_at
- **host_addresses**: host_id, kind, address, is_primary
- **ssh_access**: host_id, user, port (default 22), auth_methods (e.g. `publickey` / `publickey,password`), key_ok (bool), status (key_only / password_allowed / unreachable / unknown), last_checked_at, vault_ref
- **services**: host_id, name, type (rails/postgres/redis/nginx/caddy/docker/…), port, protocol (tcp/udp/http/https), path, domain, icon, version, eol_date, container_name, systemd_unit, compose_path, notes
- **checks**: target (service/host/domain), kind (tcp/http/tls/ssh/dns), interval, expected (status code, keyword), enabled
- **check_results**: check_id, ok, latency_ms, message, at (rolled up after 30 days)
- **projects**: name, slug, icon, description, group_id, is_open_source, runbook_md, status (active/archived)
- **project_links**: project_id, kind (repo / docs / public / secrets / other), url, label
- **project_envs**: project_id, name (staging / production / …), base_url (e.g. `https://acme.example.com`)
- **app_links** (paths inside the app, §17 #31): project_id, label, path (`/sidekiq`, `/admin`, `/mcp`…) **or** absolute url, only_env (null = all envs), kind (page / admin / jobs / db / docs / health / mcp), icon, access_id → **access** (login method + Vaultwarden item), health_check (bool), sort
- **access** (logins, §17): target (host/service/project/domain/provider), label, url, method (password / google_sso / github_sso / oidc / ssh_key / token / passkey / none), account (username or email), twofa (totp / hardware / sms / provider / none), vault_item_id (Vaultwarden deep link), notes. **No secret values.**
- **providers / provider_accounts**: name (Cloudflare, OVH, Hetzner…), account label, login email + method, panel URL, integration token ref (Clavis key)
- **domains**: name, registrar_account_id, dns_account_id, registered_at, expires_at, auto_renew, price + currency, payment_label (e.g. "Revolut ··42", never card numbers), nameservers, dnssec, transfer_lock, source (cloudflare / ovh / manual), notes
- **dns_records**: domain_id, name, type (A/AAAA/CNAME/tunnel), content, proxied, matched_host_id / matched_service_id, stale (bool), synced_at
- **costs**: target (host/domain/location/service/provider), label, amount, currency, cycle (monthly / yearly / one_time / estimate), next_renewal, auto_renew, payment_label, project_id (derived from target if empty), source (sync / manual)
- **project_hosts / project_services**: + environment

Soft delete (`deleted_at`) on all main tables; restore from UI/MCP.

---

## 4. Features by phase

### Phase 1 — Core + MCP (usable right away)
- [ ] CRUD: Locations, Hosts (+addresses), SSH access, Services, Projects (+links), Groups, Tags
- [ ] Combobox "select existing or create new" for location, group, tags, service type
- [ ] Icons: upload, or pick from bundled simple-icons / selfh.st icons, or auto-fetch favicon
- [ ] Project links: Forgejo, GitHub, public page, docs (+ open-source flag)
- [ ] Environments per project (prod/staging/dev)
- [ ] **SSH key-only check**:
  - probe auth methods with `none` auth → server returns allowed methods (no login needed)
  - try login with app key (`BatchMode`-equivalent) → key_ok
  - badge: 🟢 key-only · 🟡 password allowed · 🔴 unreachable
- [ ] **Health checks**: TCP, HTTP(S) status/keyword, latency; uptime history sparkline
- [ ] **TLS cert expiry** check
- [ ] Copy buttons (`ssh deploy@192.168.1.123 -p 2222`, URLs, IP:port)
- [ ] `~/.ssh/config` export (all hosts/users, aliases)
- [ ] ⌘K command palette + global search, favorites
- [ ] **Dashboard mode**: tile grid by group, icons, live status dot, click → open
- [ ] Markdown runbook per project/host
- [ ] Audit log (who/what/when, user vs MCP token)
- [ ] Auth: single admin password (argon2id), session cookie; API tokens (read / write)
- [ ] **MCP server** (see §6)
- [ ] Service templates (e.g. "Rails app" = web :3000 + Postgres :5432 + Redis :6379)
- [ ] Free-port suggestion when adding a service
- [ ] Dark mode, mobile-friendly (PWA manifest)
- [ ] JSON/YAML import & export

### Phase 2 — Network & Discovery
- [ ] **Proxmox import/sync** via API token: nodes, VMs, CTs, VMIDs, IPs, CPU/RAM/disk, status
- [ ] Proxmox capacity view: allocated vs available RAM/disk per node/storage
- [ ] **IPAM**: subnets, VLANs, used/free IP map, conflict warnings (IP and host:port duplicates)
- [ ] **Discovery scan** (scheduled + on demand): TCP-connect sweep of subnet on common ports
      (22, 80, 443, 3000, 5432, 6379, 8006, 8080, 9000…), banner/HTTP title grab
      → "Unregistered" inbox: register / ignore / merge into host
- [ ] **Drift detection**: open-but-undocumented ports, documented-but-closed services
- [ ] Reverse-proxy mapping: domain → proxy (edge/Caddy/nginx/Traefik) → host:port
- [ ] **DNS**: Cloudflare + OVH API read records; warn if domain doesn't resolve to expected IP; domain expiry
- [ ] Docker & systemd fields (container names, units, compose path)
- [ ] Dependencies between projects/services + network diagram (rendered SVG, e.g. via d2/mermaid-like layout)
- [ ] Forgejo / GitHub integration: last commit, CI status, latest release, open issues
- [ ] Router integration (optional later): MAC / DHCP leases (OPNsense / MikroTik / UniFi)

### Phase 3 — Operations & Alerts
- [ ] **ntfy alerts**: check down/up, cert/domain expiry, SSH password-auth detected, new device discovered
      - per-rule priority/tags, dedupe + flap protection, recovery messages
      - maintenance windows (mute per host/service/global)
      - weekly digest (expiring certs, pending updates, reboots needed, new unregistered hosts)
- [ ] **Facts over SSH** (agentless, read-only `inventory` user): OS, kernel, uptime, disk %, pending updates,
      reboot-required, failed systemd units, docker ps
- [ ] **Security flags**: PermitRootLogin, PasswordAuthentication (from `sshd -T`), firewall status, fail2ban
- [ ] **authorized_keys inventory**: which key fingerprints are on which host/user; flag unknown/stale keys
- [ ] Backups: target, schedule, last success, last verified (manual or via PBS/vzdump API)
- [ ] Versions + EOL warnings (endoflife.date data, cached)
- [ ] Cost tracking per location/host (monthly totals)
- [ ] Wake-on-LAN, `ssh://` open-in-terminal links
- [ ] Saved commands/snippets per host
- [ ] Timeline per host ("upgraded PG 16→17")
- [ ] Public status page (opt-in services, read-only)
- [ ] Exports: Ansible inventory, Prometheus file_sd targets, `/metrics` endpoint

---

## 5. Checks & scheduler

- Single in-process scheduler; each check has interval (default 60 s TCP/HTTP, 6 h TLS, 1 h SSH, 24 h discovery).
- Worker pool with concurrency limit (default 32) and per-host rate limit.
- Results kept raw 30 days, then hourly rollups 1 year.
- State machine per check: `up → failing (n consecutive) → down → recovering → up` → drives alerts.

---

## 6. MCP server

- Transport: **Streamable HTTP** at `/mcp`, `Authorization: Bearer <token>`.
- Token scopes: `read` (list/get/search) and `write` (create/update/delete/run checks).
- Every write is audit-logged with the token name; deletes are soft (restorable).

Tools:

| Tool | Purpose |
|---|---|
| `search` | free-text across projects/hosts/services/IPs/domains |
| `list_projects` / `get_project` | with links, hosts, services, status |
| `list_hosts` / `get_host` | with addresses, SSH access, services, facts |
| `list_locations` / `list_groups` / `list_tags` | lookup values |
| `create_project` / `update_project` / `delete_project` | |
| `create_host` / `update_host` / `delete_host` | |
| `add_service` / `update_service` / `remove_service` | |
| `add_ssh_access` / `remove_ssh_access` | |
| `link_project` | attach host/service to project (+ environment) |
| `add_link` | forgejo/github/public/docs link |
| `check_ssh` / `check_service` | run check now, return result |
| `suggest_free_port` / `find_free_ip` | |
| `list_discovered` / `register_discovered` | discovery inbox |
| `add_timeline_entry` | log a change ("deployed v1.4") |
| `restore` | undo soft delete |
| `list_access` / `add_access` | logins for an item: method, account, URL, Vaultwarden item **name** — never secret values |
| `list_app_links` / `add_app_link` | project paths per environment (`/sidekiq`, `/admin`, `/mcp`) → full URL + login method; lets an agent find e.g. a project's MCP endpoint |
| `list_domains` / `get_domain` | registrar, account, expiry, auto-renew, price, records |
| `list_costs` | recurring + one-time costs, totals by provider / project |

Resources: `homeservices://project/{slug}`, `homeservices://host/{id}` (markdown summaries).

Example Claude Code config:
```bash
claude mcp add --transport http homeservices http://<app-ip>:8080/mcp --header "Authorization: Bearer <token>"
```

---

## 7. Secrets policy

- Never store passwords or private keys of target systems in the DB.
- Store **references** only: a Vaultwarden item link (human passwords) or a secret-manager key name (app secrets).
- The app's own secrets (Proxmox, Cloudflare, OVH, ntfy, Forgejo, router tokens, admin password hash,
  session key) can be injected by an external secret manager at start-up. Only the manager's own
  machine token is stored locally (0600); everything else arrives as environment variables.
- If the manager uses a self-signed certificate, install its CA in the app host's trust store.
- Fallback: if the manager is unavailable at boot, the app still serves the UI and simply refuses
  to use integrations — no secrets are cached on disk.
- The app's SSH private key stays on disk (`/etc/homeservices/ssh/id_ed25519`, 0600) — SSH needs it as a file.
- **Logins for web/SSH/DBs: Vaultwarden links only** (decided 2026-11-16). The app stores method, account, URL,
  2FA type and the Vaultwarden item id → "Open in Vaultwarden" button. Vaultwarden is end-to-end encrypted, so the
  app could not read it anyway without your master password.
- **AI agents never read passwords** (for now). MCP returns method / account / item name only. There is no
  `secrets:read` scope; revisit later if needed (would require per-request approval via ntfy + audit).
- Integration tokens entered in the UI are encrypted in `data/secrets.enc` (keyed by `HS_MASTER_KEY`), not
  stored in SQLite.

---

## 8. Deployment — LXC on Proxmox

The shipped path is one static binary on a Debian/Ubuntu LXC or VM under systemd. `deploy/install.sh`
creates the service user, data directory, env file and units; `deploy/update.sh` backs up, checks
migrations, swaps the binary, restarts and rolls back on a failed health check. Backups, restore and
the full environment-variable table are in `deploy/README.md`.

Environment-specific notes (real CT IDs, addresses, inventory and routes) live in the private
deployment repository, not here.

---

## 9. Integration credentials

Integrations keep credentials in the encrypted `SecretStore` (`HS_MASTER_KEY`) or accept them on
Settings › Integrations. A typical setup needs:

| Integration | What to provide |
|---|---|
| Proxmox | API URL and a read-only token |
| Caddy | admin API URL (read-only `GET /config/`) |
| Cloudflare | API token with read-only zone/DNS/tunnel/registrar permissions |
| Docker over SSH | `user@host` targets with read-only Docker access |
| ntfy / Gotify / webhook | topic or endpoint URL and an optional token |
| Home Assistant / Shelly / Tasmota | base URL and a read-only token |

None of these secrets are stored in SQLite.

---

## 10. Project layout

```
cmd/homeservices/main.go
internal/
  config/        config + secrets loading
  db/            migrations (goose), sqlc queries, models
  web/           handlers, templ views, htmx partials, static (embedded)
  auth/          sessions, api tokens
  checks/        tcp, http, tls, ssh, dns probes + scheduler
  discovery/     subnet sweep, drift
  integrations/  proxmox, forgejo, github, cloudflare, ovh, ntfy
  mcp/           MCP tools/resources
  audit/
deploy/
  homeservices.service
  install.sh  update.sh  bootstrap-host.sh
Makefile
PLAN.md
```

---

## 11. Milestones

> Superseded by the detailed, re-ordered task list in [IMPLEMENTATION.md](IMPLEMENTATION.md) (MCP moved to M3). Kept for history.

1. **M1 – Skeleton**: Go module, chi, templ, htmx, SQLite + migrations, auth, layout, dark mode.
2. **M2 – Inventory CRUD**: locations, hosts, addresses, SSH access, services, projects, links, groups, tags, icons, comboboxes, soft delete, audit log.
3. **M3 – Checks**: scheduler, TCP/HTTP/TLS/SSH checks, status badges, uptime sparkline.
4. **M4 – UX**: ⌘K search, dashboard mode, copy buttons, ssh config export, templates, free-port suggestion, import/export.
5. **M5 – MCP**: tokens with scopes, all tools, resources, docs.
6. **M6 – Deploy**: systemd unit, install/update scripts, a Debian LXC/VM behind a reverse proxy, backups.
7. **M7 – Phase 2** (Proxmox import, IPAM, discovery, drift, DNS, Forgejo/GitHub, dependencies diagram).
8. **M8 – Phase 3** (ntfy alerts + digest, SSH facts, security flags, authorized_keys, backups, EOL, costs, WoL, status page, exports).

---

## 12. Open questions

- Public hostname for the app via `edge` (e.g. `homeservices.example.com`) or LAN-only?
- ntfy topic name and whether alerts should go to one topic or per-severity topics.
- Other subnets besides `192.168.1.0/24` and `192.168.2.0/24` (Tailscale, OVH private network)?
- Add the static route `192.168.2.0/24 via 192.168.1.151` on the main router `192.168.1.1`
  (one place, every device benefits) instead of per-device routes? Then `route-88.service` in the app CT becomes optional.

## 13. Decisions log

- 2026-11-16: Go + SQLite + htmx instead of Rails (resource usage).
- 2026-11-16: Dashboard mode is an extra view, not a replacement for an existing dashboard.
- 2026-11-16: The app runs in its own unprivileged Debian LXC.
- 2026-11-16: Discovery subnets are configurable; the default is `192.168.1.0/24`.
- 2026-11-16: Go over Rust — RAM difference (~20 MB vs ~8 MB) irrelevant in a 512 MB CT; Go wins on
  build speed, simplicity, mature `x/crypto/ssh`, official MCP SDK, trivial cross-compile.
- 2026-11-16: Reverse-proxy import targets the Caddy admin API (read-only `GET /config/`).
- 2026-11-16: Secrets stay outside the app: Vaultwarden links for humans, an external manager for machine tokens.
- 2026-11-16: The project lives in a private Git repository and is published with a fresh history.
- 2026-11-16: Logins = Vaultwarden links only; AI agents don't read passwords. Added Access, Domains (registrar/expiry/price), Costs.
- 2026-11-16: Will be open source. Core works with no integrations; every external tool is an optional provider plugin (§18). Maintainer's stack = first plugins. License TBD (AGPL-3.0 recommended).
- 2026-11-16: Store queries stay hand-written for v0.1; sqlc would add a codegen step for a small schema. Revisit if `internal/inventory` passes ~2 k lines (it is ~1.2 k now).
- 2026-11-16: UI is `html/template` with a shared layout + per-page content template (no codegen step); htmx 2.0.4 is vendored into `static/` and embedded, never loaded from a CDN. Revisit `templ` only if templates outgrow the layout. Prototype `app.css` / `boot.js` / `themes.js` are reused; `app.js` stays with the prototype screens.

## 14. Design implications from a first deployment

A real deployment confirmed the feature order:

- a **Caddy importer** (admin API → domain → upstream → host/service) is the quickest way to map
  proxy routes; flag routes whose upstream is down or unknown;
- a **public-exposure flag** for services reached through a tunnel or reverse proxy, with an alert
  when a newly exposed service accepts SSH passwords or has no auth;
- **secret-manager references** instead of secrets in the app (Vaultwarden for humans, Clavis or
  another manager for machine tokens);
- **router integration** (DHCP leases / ARP) to enrich discovery across subnets;
- **stale-resource and duplicate-name views** — stopped VMs/CTs and duplicate names are common;
- **Docker container inventory** over SSH (`docker ps`) for compose stacks;
- **stale route detection** when a proxy route or DNS record points at an address that no longer
  answers;
- **resource-hog and node-capacity views** (RAM/disk per node) with threshold alerts.

---

## 15. More features (added 2026-11-16)

| # | Feature | What it does | Phase |
|---|---|---|---|
| 14 | **Decommission checklist** | Removing a host lists everything still pointing at it — DNS records, Caddy routes, backups, Proxmox guest, SSH logins, Vaultwarden/Clavis refs, project links — with a tick-off checklist; host keeps an archive record | 2 |
| 15 | **SSH fix guide** | For hosts allowing password login: exact copy-paste commands (add key → test → `PasswordAuthentication no` → reload), per OS (Debian, Alpine, RouterOS). App never changes servers itself; re-check button verifies | 1 |
| 16 | **MAC vendor names** | Offline OUI database → unknown DHCP clients show "Apple", "Raspberry Pi", "TP-Link"… in Discovery and Network map | 2 |
| 17 | **Lifecycle status** | `planned → active → deprecated → decommissioned` for hosts, services, projects; filters + dashboard hides decommissioned | 1 |
| 18 | **Incident notes** | Each down→up episode becomes an incident; add a note ("OOM, raised memory"); shown on host/service timeline | 3 |
| 19 | **Expiry calendar** | One list/calendar of TLS certs, domains, OVH renewals, hardware warranties, token expiries; ntfy reminders 30/7/1 days before | 3 |
| 20 | **Topology map** | Internet → Cloudflare → edge (Caddy) → hosts → services; MikroTik → 88.x; built from routes, addresses and links | 2 |
| 21 | **`/inventory.md` for AI** | Read-only Markdown export of the whole inventory (token-protected) so any agent can read context without MCP | 1 |
| 22 | **Physical devices** | Hardware model, serial, purchase date, warranty end, location in rack/room — for MikroTik, Pis, NAS, Proxmox box | 2 |
| 23 | **Keyboard shortcuts** | `g d/h/p/s/n/a` navigation, `n` new, `/` or ⌘K search, `?` shows all shortcuts | 1 |

New screens: Settings (integrations, MCP tokens, ntfy, scanning, account), Alerts (open, history, rules, maintenance),
Services & ports, Domains & expiry calendar, Topology, Decommission, First-run setup.

## 16. UX principles

1. **Plain words first** — "Login with keys only" not "PasswordAuthentication no"; technical detail one click away.
2. **Every problem has a next step** — each alert/badge has a *Fix* or *Explain* action (drawer with guidance), never a dead end.
3. **Paste anything** — Quick add accepts `ssh root@192.168.1.5 -p 2222`, `https://app.lan:8443`, `192.168.1.5:5432`, a Proxmox VMID… and shows what will be created before saving.
4. **Pick, don't type** — locations, groups, tags, credentials refs are comboboxes with "create new" inline.
5. **Copy on click** for every IP, port, URL and command.
6. **Undo instead of "are you sure?"** for edits and soft deletes; confirmation only for real-world effects.
7. **Explain jargon** — small `?` tooltips on drift, key-only, lifecycle, etc.
8. **Good empty states** — first run is a guided setup (Proxmox import → subnet scan → ntfy) instead of empty tables.
9. **Keyboard + mobile** — full keyboard navigation on desktop, bottom tab bar and large tap targets on phone.
10. **Calm dashboard** — one-sentence status ("All good" / "2 things need attention") above the details.

## 17. Access, domains & costs (added 2026-11-16)

| # | Feature | What it does | Phase |
|---|---|---|---|
| 24 | **Access card** | On every host / project / domain: each login with URL, method (password, Google/GitHub sign-in, SSH key, token, none), account, 2FA, and an *Open in Vaultwarden* button. Shows "no login" warnings (e.g. Mailpit open on LAN) | 1 |
| 25 | **Accounts view** | Which email / Google account is used where — "what breaks if I lose `x@gmail.com`?" | 3 |
| 26 | **Cloudflare import** | One read-only token per account imports **all** zones, DNS records, tunnel routes and registrar data (expiry, auto-renew, price). Tunnel ingress `hostname → 192.168.1.x:port` auto-links records to hosts/services; A records match by IP; CNAMEs are followed. New domains appear on their own. Unmatched / stale records get their own filter with *link to host* / *clean up* | 2 |
| 27 | **OVH import** | Domains, VPS, renewal dates and prices from the OVH API | 2 |
| 28 | **Domain page** | Registration (bought at, account, dates, price, payment label, nameservers, DNSSEC, lock), records, account/logins, cost history, used by | 2 |
| 29 | **Costs page** | Recurring (monthly/yearly) + one-time hardware; per month / per year toggle; by provider and by project; next 30 days; "not used by anything" flag; free plans listed; CSV export. Power estimate from Proxmox avg load × W × €/kWh | 3 |
| 30 | **Renewal reminders** | Domains/VPS renewals in the expiry calendar; ntfy at 30/7/1 days; extra nudge when auto-renew is **off** ("renew or let go?") | 3 |
| 31 | **App links** | Per project: one list of paths (`/`, `/admin`, `/sidekiq`, `/pghero`, `/api-docs`, `/up`, `/mcp`) joined to the selected environment's base URL — Staging/Production switch, copy full URL, Open. Each link shows its login (basic auth / email+password / bearer / none + *Open in Vaultwarden*); env-only links (Mailpit on staging). Health and Sidekiq stats where a check exists. Suggestions from `config/routes.rb` (Forgejo) or Caddy routes. Also searchable in ⌘K ("acme sidekiq" → opens it) | 1 |
| 32 | **MCP endpoints** | App links of kind `mcp` get a *Connect an agent* drawer: Vaultwarden link for the token, hidden `read -s` to load it, ready `claude mcp add --transport http …` command for the chosen env, `claude mcp list` to check | 1 |
| 33 | **Connected services** | Per project: tiles for the outside services it uses — Forgejo (code/CI), GitHub (mirror), Docker registry (images), Clavis (secrets, project/env/key count), Vaultwarden (logins folder), ntfy (topic), OVH/Cloudflare (domain, VPS), BookStack (docs), S3/SeaweedFS (bucket). Each: role, one-line detail, status dot, click opens it, `!` tooltip "if X is down: …" (impact). Suggestions from Clavis key names (e.g. `SENTRY_DSN` → add Sentry?) and compose/env files | 1 |
| 34 | **Deploy pipeline view** | Read-only diagram from `.forgejo/workflows/*.yml` (and compose / `clavis run` on the target): push → Forgejo Actions → registry → `deploy@host` → health check → ntfy, with the last run status/duration and *View run*. Home Services never triggers deploys | 2 |
| 35 | **Reverse "Used by"** | On each shared service's page (Clavis, registry, Forgejo, ntfy…): which projects depend on it and what breaks if it's down — answers "can I restart Clavis now?" | 2 |

Data: **project_connections**: project_id, target (host / service / provider / external URL), role (code / ci / mirror / images / secrets / logins / alerts / dns / domain / docs / storage / monitoring / other), detail, url, impact_note, access_id, source (manual / detected).

Making Cloudflare simple: user never types records. Flow = *Connect Cloudflare* drawer (open token page → tick 4 read
permissions → paste → **Test token** shows what will be imported) → sync every 15 min. Several accounts = several
tokens; one merged list labelled by account. Manual *Add by hand* only for registrars without an API.

New screens: `domain.html`, `costs.html`; reworked `domains.html` (providers strip, registered domains, records with match status).

## 18. Open source — no lock-in to anyone's tools (added 2026-11-16)

Goal: publish Home Services so anyone can run it, whatever they use. The maintainer's stack (Proxmox, Caddy,
Cloudflare, OVH, Forgejo, ntfy, Vaultwarden, Clavis, Gatus, MikroTik) is just **one set of plugins**, not built in.

**Principles**
1. **Works with zero integrations.** Manual entry, Quick add / paste, subnet scan, SSH key-only check, HTTP/TCP/TLS
   checks, in-app alerts, expiry calendar, costs, MCP, `/inventory.md` — all core, no external service needed.
2. **Everything external is a provider** behind a small Go interface, grouped by category:

   | Category | Interface | Shipped first | Easy next |
   |---|---|---|---|
   | Servers & containers | `Inventory` (list guests, IPs, state) | Proxmox VE, Docker | Incus/LXD, Portainer, TrueNAS, Unraid |
   | Router & DHCP | `LeaseSource` | MikroTik | OpenWrt, OPNsense/pfSense, UniFi, Pi-hole, AdGuard Home |
   | Reverse proxy | `RouteSource` (domain → host:port) | Caddy, Traefik | Nginx Proxy Manager, nginx files, HAProxy |
   | DNS & domains | `DNSProvider`, `Registrar` | Cloudflare, OVH | Porkbun, Namecheap, Hetzner, Route 53, deSEC |
   | Code & CI | `GitHost`, `CIProvider` | Forgejo/Gitea, GitHub | GitLab, Woodpecker, Drone |
   | Notifications | `Notifier` | ntfy, Webhook, Email (SMTP) | Gotify, Pushover, Telegram, Discord, Slack, Apprise |
   | Password manager | `PasswordManager` — **links only** (`DeepLink(itemID)`), no read method exists | Bitwarden/Vaultwarden, none | 1Password, KeePassXC (item name only), Proton Pass, Passbolt |
   | Secret store (app's own tokens) | `SecretStore` (Get/Put) | **Built-in encrypted file**, env vars | Clavis, a previous secrets manager/Vault, Infisical, SOPS, Doppler |
   | Monitoring import | `MonitorImport` | Gatus, Uptime Kuma | Prometheus, Healthchecks, Zabbix |

   Each provider self-registers (`integrations.Register`), declares a config schema (→ auto-generated Settings form
   and *Connect* drawer steps), and implements `Test()` + `Sync()`. Adding one = one Go file + test + docs page.
3. **No hard-coded product names in the UI.** Labels come from the configured provider ("Open in 1Password",
   "Tokens are stored in a previous secrets manager"). Settings → Integrations is a catalog: categories, connected ones ticked, "Other…".
4. **Generic escape hatches:** Webhook notifier, JSON/YAML import + export, generic HTTP check, `/inventory.md`, MCP.
5. **App secrets default to a built-in store**: tokens encrypted with age/XChaCha20 in `data/secrets.enc`, key from
   `HS_MASTER_KEY` env or systemd credential. Clavis (maintainer's choice) / a previous secrets manager / Infisical are optional
   `SecretStore` plugins. (§7 describes the maintainer's own deployment.)
6. **Packaging:** single static binary (linux amd64/arm64), multi-arch Docker image + `compose.yaml`, systemd unit,
   Proxmox LXC helper script — LXC is one option, not a requirement.
7. **Privacy by default:** no telemetry, no outbound requests unless an integration is configured; favicon fetching
   and update checks can be turned off.
8. **Demo mode:** `homeservices --demo` seeds fictional data (`example.com`, `10.0.0.0/24`, "acme" projects) for
   screenshots and a public demo.
9. **Setup wizard asks "What do you already use?"** (`onboarding-tools.html`) — everything optional, only picked
   tools get connect steps.

**Repo hygiene before publishing**
- The current repo contains the maintainer's real environment (IPs, hostnames, domains in §2/§14 and the design
  mockups). No secrets, but it shouldn't be public.
- Plan: public repo gets generic docs + demo data; personal deployment notes (§2, §8, §9, §14) move to a private
  `home-services-deploy` repo on Forgejo; mockups switch to demo data (keep a private copy with real data).
- Publish with fresh history (squash) so old commits don't expose the home network layout.
- Add `README` (screenshots from demo mode), `CONTRIBUTING.md` (› Adding an integration), `SECURITY.md`,
  `LICENSE`, issue templates; i18n-ready strings.

**License (decision pending):** AGPL-3.0 (common for self-hosted apps; changes to a hosted fork must be shared) vs
MIT/Apache-2.0 (simplest, most permissive). Recommendation: **AGPL-3.0**.

## 19. Plugin recognition & AI-assisted setup (added 2026-11-16)

**Recognise → suggest a plugin.** Each plugin ships a *fingerprint* file (no code needed to add one):
ports, SSH/TCP banners, HTTP page title, favicon hash, response headers, API probe paths (`/api/v1/version`,
`/manifest.json`), mDNS names, and product-name aliases. Matched in three places:
1. **Discovery scans** → "Suggested plugins" list with the evidence ("SSH banner ROSSSH · port 8728") and what you'd
   get. Actions: *Connect* / *Turn on* (no token when it can reuse an existing SSH login, e.g. Docker) / *Not now*.
2. **Quick add** — paste `ip:port` or a URL → "Looks like Uptime Kuma — connect to import its monitors?".
3. **Typed names** — "uptime kuma" → knows the product, offers to scan subnets for its default port, then suggests the plugin.

Fingerprints run locally; nothing is sent outside the network. Plugins without code (community) can still provide
fingerprints + a service template (icon, default ports, health path, app links like `/admin`).

**AI-assisted setup (MCP).** An agent documents the infrastructure; the human approves.
- Agent modes (global and per token): **Only read** · **Propose** (default) · **Write directly** (still undoable).
- `propose_changes` creates a *changeset*: list of +/~/− operations, each with **evidence** (file + line, scan
  result) and a confidence; low-confidence ones arrive unticked. Review screen `ai.html`: tick/untick, comment,
  *Apply selected*, *Reject*; history with **Undo per changeset**. Bell + dashboard show "AI proposed N changes".
- Agents **never** see or pass secrets: a step that needs a token becomes "you paste it here"; Vaultwarden items are
  picked by the human. Hard deletes are never available to agents.
- Home Services **never runs commands on servers**. For real infra changes the agent uses its own tools (SSH, git);
  Home Services provides context, fix guides and runbooks, then re-checks and records the result.
- Starter prompts on the page: inventory my network, document a project from its repo, find what's missing, plan a new service.

New MCP tools:

| Tool | Purpose |
|---|---|
| `detect` | address / URL / name → product, confidence, evidence, matching plugin |
| `list_plugins` / `suggest_plugins` | installed + available plugins; suggestions from last scan |
| `propose_changes` | submit a changeset (ops + evidence + confidence) for review |
| `get_changeset` | status: pending / applied / rejected (+ human comment) |
| `request_connection` | ask the human to connect a plugin; returns a link where they paste the token |

Data: **changesets** (id, token_name, title, prompt, status, created/decided_at, comment) and **changeset_ops**
(changeset_id, op add/update/delete, entity, payload JSON, evidence, confidence, selected, applied_audit_id).

## 20. More plugin categories & home page (added 2026-11-16)

New categories in the catalog (same plugin interface; fingerprints where a product is detectable on the network):

| Category | What it adds to Home Services | Candidates |
|---|---|---|
| Cloud & VPS | servers, IPs, monthly cost → Costs page | Hetzner Cloud, OVH, DigitalOcean, Linode, Scaleway, Lightsail, Oracle |
| Backups | last good backup per host, size, failures, last restore test; alert if older than N days | Proxmox Backup Server, restic, Borg/Borgmatic, Kopia, Backrest, Duplicati |
| Updates | pending OS updates, newer image tags, EOL versions (endoflife.date) | apt/dnf/apk over SSH, Docker tags, Diun, What's Up Docker, Renovate |
| VPN & remote access | remote addresses, who can reach what from outside | Tailscale, Headscale, WireGuard, NetBird, ZeroTier, Cloudflare Tunnel |
| Login / SSO | which apps use SSO → fills Access cards automatically | Authentik, Authelia, Keycloak, Pocket ID, Zitadel |
| Metrics & logs | CPU/RAM/disk on host pages, links to logs | Prometheus, Netdata, Beszel, Glances, Loki, Dozzle |
| Storage & hardware | disk SMART health, pool usage, UPS battery, IPMI links | TrueNAS, Synology, ZFS over SSH, smartctl, NUT, IPMI/iDRAC/iLO |
| Power & smart home | real power per device → electricity cost instead of estimate | Home Assistant, Shelly, Tasmota, Kasa |
| Export & sync (outbound) | one source of truth → generate other tools' config | Homepage `services.yaml`, Homarr, Dashy, Ansible inventory, `~/.ssh/config`, NetBox, Uptime Kuma monitors |

Home page (dashboard) order: greeting + one status sentence → attention bar → compact numbers strip (each links
to its page) → tiles with their own filter bar (groups, Pinned, tiles/list, ⋯ customize, wall display mode).

Sidebar: collapsible to an icon rail (button left of breadcrumbs, key `[`), remembered in localStorage; with no saved
choice it starts collapsed below 1200 px. In the Go app the choice also goes to the user's settings so it follows them
across browsers.

## 21. Home views & Power plugin (added 2026-11-16)

- **Two home views, user's choice, remembered** (localStorage now, user setting in the Go app):
  *Tiles* (`dashboard.html` — status, attention, numbers, detailed tiles) and *Launcher* (`dashboard-launcher.html` —
  big app icons, "open anything" search, attention/recent side panel). Switcher at the top of both; the sidebar
  "Dashboard" link and logo go to the chosen one. Only the home page differs — every other page is shared.
- **Electricity is a plugin, not core.** Costs works without it (bills, renewals, hardware). The *Power* plugin adds
  an electricity row on Costs and live watts on host pages. Sources, in order of preference:
  1. **Home Assistant** — map hosts to existing power sensors (`sensor.*_power`), optional `sensor.electricity_price`;
     read-only long-lived token. Detected automatically when HA is on the network. Many self-hosters run HA, but it's
     just the first source, not a requirement.
  2. Smart plug directly (Shelly, Tasmota, Kasa).
  3. UPS (NUT) or server management (IPMI / iDRAC / iLO).
  4. Estimate (machine type × CPU load) or manual watts from a plug-in meter.
  Price per kWh: from the bill, HA sensor, or country average. Uninstalling removes the row; nothing else depends on it.

## 22. Themes — user-made, shareable through the repo (added 2026-11-16)

**What a theme can change:** colors (separate dark and light sets), font, corner radius, spacing (compact / normal /
roomy), page layout defaults (home view Tiles/Launcher, sidebar expanded/icons, availability bars on/off) and
optional custom CSS. **What it can't:** run scripts, load fonts/images/CSS from the internet, or change behaviour.

**Format** — one folder per theme in the repo:
```
themes/nord/theme.json     # required
themes/nord/theme.css      # optional, same rules as the editor's custom CSS
themes/nord/preview.png    # optional, generated by `make theme-previews` if missing
```
```json
{ "id": "nord", "name": "Nord", "author": "…", "version": "1.0.0", "license": "MIT",
  "modes": { "dark": { "--bg": "#2e3440", "--accent": "#88c0d0" }, "light": { "--bg": "#eceff4" } },
  "font": "inter|system|rounded|mono|serif", "radius": 10, "density": "compact|normal|comfy",
  "layout": { "home": "tiles|launcher", "sidebar": "open|collapsed", "bars": true }, "css": "" }
```
Tokens = the CSS variables in `app.css` (`--bg`, `--bg-2`, `--panel`, `--panel-2`, `--hover`, `--border`, `--text`,
`--text-2`, `--muted`, `--accent`, `--ok`, `--warn`, `--bad`, `--violet`, `*-soft` derived automatically). The
token list is the public theming API — renaming one is a breaking change (themes declare `"api": 1`).

**Safety:** CSS is sanitised server-side on save/import (no `@import`, no `url()` except `data:` and bundled
assets, no `expression()`), and the app sends a strict CSP (`style-src 'self' 'unsafe-inline'`, `img-src 'self' data:`,
`connect-src 'self'`). Only admins can install themes. Contributed themes go through PR review + a CI check that
validates `theme.json` against the schema, runs the sanitiser, and fails on WCAG contrast < 4.5:1 for body text.

**UI:** Settings → Appearance (gallery with live mini previews; *Use*, *Edit a copy*, *Import*, link to community
themes) and `theme-editor.html` (controls + live preview of the whole app, contrast badges, *Save & use*, *Export*
→ `theme.json` ready for a PR). Theme is stored per user; boot script applies it before first paint (no flash).

Built-in presets: Default, High contrast. Example community themes shipped in `themes/`: Nord, Solarized, Terminal, Sunset.

## 23. Authentication is a plugin (decided 2026-11-16)

UI auth is an `auth.Provider` selected by `HS_AUTH`. Default **`none`** (trusted LAN, no login). Implemented: `password`
(argon2id hash from `homeservices hash-password`, HMAC-signed 30-day cookie, `/healthz` and `/mcp` stay reachable).
Planned providers: `proxy-header` (Cloudflare Access / Authelia / Authentik forward-auth), `oidc`.
**MCP/API tokens are independent of UI auth** — they always apply, so write access stays gated even with `none`.
With `none`, don't expose the port to the internet; bind to the LAN or put it behind an access proxy.
**What `none` really means (2026-11-16 test pass):** the browser UI is open to anyone who can reach the port, and
the UI can write — so a LAN client can create/delete records through the forms (or by sending `Sec-Fetch-Site:
same-origin` to `/api/v1`). Tokens protect *agents and scripts*, not the network. What `none` does guarantee: other
**websites** can't use your browser against the app — every POST/PUT/PATCH/DELETE goes through Go's
`http.CrossOriginProtection`, and redirects only go to local paths. Use `password` (or an access proxy) whenever
the port is reachable from outside your own LAN.
