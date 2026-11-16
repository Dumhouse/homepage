# Home Services — Implementation plan & tasks

The working task list for the Go app. `PLAN.md` says **what** and **why**; this file says **in what order, which
files, and when a task counts as done**. Tick boxes as tasks land; one commit per task (or per small group) is ideal.

Status legend: `[x]` done · `[~]` in progress · `[ ]` todo. IDs (e.g. `M3.4`) are used in commit messages.

---

## 0. Ground rules

- **One binary, no build step for the UI.** Go + embedded SQL migrations + embedded templates/assets.
- **Every write goes through `inventory.Store`** (or a sibling store) inside a transaction with an `audit_log` row.
  HTTP API, HTML pages and MCP are thin adapters over the same store — no SQL in handlers.
- **Soft delete everywhere**; restore is a first-class operation. Hard delete only from Settings › Trash (human only).
- **No secrets in SQLite.** Logins = method/account/Vaultwarden link. App tokens = hashed. Integration tokens = `SecretStore`.
- **Tests per layer:** store tests on `:memory:` SQLite, API tests through `httptest`, MCP tests through the SDK's
  in-memory transport. `make test` must stay green and under ~10 s.
- **Migrations are append-only** (`0002_…sql`, `0003_…sql`); never edit a shipped one.
- **Design prototype stays** in `design/` as the visual spec until each page has a real replacement; replaced pages
  are removed from the prototype router, not from the repo, until M4 ends.

Definition of done for any task: code + tests + `go vet` clean + the behaviour checked once against a running binary
+ the relevant `PLAN.md`/this file updated.

---

## Overview — order of milestones

| # | Milestone | Outcome | Size |
|---|---|---|---|
| M0 | Design prototype | Clickable HTML spec of every screen | ✅ done |
| M1 | Skeleton | Go server, SQLite, migrations, auth plugin | ✅ done |
| M2 | Inventory core | All core entities with store + JSON API | ✅ done |
| M3 | Tokens + MCP | AI agents can read/write the inventory | ✅ done |
| M4 | Real pages | Server-rendered UI replaces the prototype for core screens | ✅ done (drawers optional) |
| M5 | Checks | TCP/HTTP/TLS/SSH key-only checks, scheduler, status everywhere | ✅ done |
| M6 | Deploy | Runs in a Debian LXC under systemd, backups, update script | ✅ done |
| M7 | Discovery & plugins | Plugin framework, subnet scan, Proxmox + Cloudflare import | ✅ done |
| M8 | Alerts & ops | ntfy alerts, expiry calendar, costs, domains, SSH facts | ✅ done |
| M9 | AI changesets | `propose_changes` → review queue → apply/undo | ✅ done |
| M10 | Open-source release | Demo mode, docs, Docker image, fresh public repo | ✅ done |

**Order change vs `PLAN.md` §11:** MCP moves from M5 to **M3** — it's the cheapest way to fill the inventory
(an agent documents the network while real pages are being built) and it reuses the store directly.

The first usable cut ("v0.1, I use it daily") = **M2 + M3 + M4 (home, host, project) + M5 (TCP/HTTP/SSH) + M6**.

---

## M0 — Design prototype ✅

- [x] All screens in `design/` (home tiles + launcher, hosts, host, projects, project, domains, domain, costs,
      discovery, AI review, settings, theme editor, onboarding).
- [x] Theme engine (`boot.js`, `themes.js`), custom theme format (PLAN §22).
- [x] Connect-an-agent drawer for Claude Code, Codex, Gemini, Cursor, VS Code, stdio bridge.

## M1 — Skeleton ✅

- [x] `go.mod`, chi router, security headers + CSP, `/healthz` (pings DB). — `internal/web/server.go`
- [x] SQLite (modernc, WAL, FK on, single writer), goose migrations embedded. — `internal/db`
- [x] Prototype embedded and served at `/`. — `embed.go`
- [x] Auth as a plugin: `none` (default), `password` (argon2id, HMAC cookie, `hash-password` cmd). — `internal/auth`
- [x] `Makefile`: `run`, `test`, `build` (static, `CGO_ENABLED=0`).

---

## M2 — Inventory core

Goal: every entity the core screens need exists in the store, with validation, soft delete, audit and JSON API.

### Done
- [x] **M2.1** Hosts with addresses (auto-primary) and SSH logins (default port 22, status `unknown`).
- [x] **M2.2** Services on hosts (port/protocol validation).
- [x] **M2.3** Projects (auto slug, unique), soft delete + restore, audit log for all writes.
- [x] **M2.4** `/api/v1` for hosts, services, projects; 400/404/422 mapping; strict JSON decoding.
- [x] **M2.5 Locations** — CRUD + `host.location_id`; deleting a location in use → 422; `hosts` JSON includes
      `location {id,name}`. `/locations` lists places with host counts that link to `/hosts?loc=…`, plus new/edit
      forms and delete/restore. — `inventory/locations.go`, `web/pages_locations.go`
- [x] **M2.6 Tags & groups as real tables** — `0002_tags_groups.sql`; taggings for hosts and services, hosts' `tags`
      column migrated into taggings; `projects.group_id` migrated from `group_name`; API accepts tag and group
      **names** and creates missing ones; `GET /api/v1/tags` returns names with usage counts. — `inventory/tags.go`
- [x] **M2.7 Project links** — `project_links(project_id, kind repo|docs|public|secrets|other, url, label, sort)`,
      included in project JSON, absolute http(s) validation. — `inventory/projects.go`
- [x] **M2.8 Environments & app links** — `project_envs` + `app_links`; envs resolve `base_url + path`;
      `only_env` links appear in one env only; absolute override URL supported. — `inventory/projects.go`
- [x] **M2.9 Access (logins)** — `access(target_entity,target_id,label,url,method,account,twofa,vault_item_url,notes)`;
      no secret field; method/2FA enums; Vaultwarden link must be http(s). — `inventory/projects.go`
- [x] **M2.10 Project ↔ host/service links** — `project_hosts`/`project_services` with env; host JSON lists its
      projects, project JSON lists hosts/services per env.
- [x] **M2.11 Connected services** — `project_connections` with role/detail/impact/access/source;
      `GET /api/v1/used-by?target_kind=&target_id=` answers "what breaks".

### Todo
- [x] **M2.12 Search** — `Store.Search(q)` across names, hostnames, addresses, service names/domains, project
      slugs, app-link labels and locations; ranked (exact > prefix > contains), max 20, typed
      `{kind,id,title,subtitle,url}`; app-link URLs resolved against the first env. `GET /api/v1/search?q=`.
      (LIKE/in-memory matching instead of FTS5 — the home-lab dataset is small; revisit if it grows.) — `inventory/search.go`
- [x] **M2.13 Conflict checks** — warn (not block) on duplicate IP across hosts and duplicate `host:port`; returned
      as `warnings[]` in create/update responses. `GET /api/v1/hosts/{id}/free-port?near=3000` skips ports used on
      the same addresses. — `inventory/conflicts.go`
- [x] **M2.14 Import/export** — `GET /api/v1/export` (JSON, all live entities, embedded services/access/children)
      and `POST /api/v1/import?mode=merge|replace` (dry-run by default; `apply=1` applies). Merge matches locations
      by name, hosts by name and projects by slug; replace wipes inventory rows (audit log and tokens stay).
      Old ids are remapped for project links, app-link access and connection targets. — `inventory/export.go`
- [x] **M2.15 Audit API** — `GET /api/v1/audit?entity=&id=&limit=` for host/project timelines. — `inventory/audit.go`
- [x] **M2.16 sqlc decision** — keep hand-written SQL; recorded in PLAN §1 and §13. Revisit past ~2k lines.

Tests: each task adds store tests (validation, soft delete, restore, audit row) + one API round-trip.

---

## M3 — API tokens + MCP server

Goal: `claude mcp add --transport http homeservices http://…/mcp --header "Authorization: Bearer …"` works and an
agent can document a host end-to-end. Tokens gate `/mcp` and `/api/v1` **regardless of UI auth**.

- [x] **M3.1 Token store** — `internal/tokens`: `Create(name, scope) → plaintext` (shown once, format
      `hs_<12 id>_<32 secret>`, stored as SHA-256 of the secret — high-entropy, no argon2 needed), `Verify(header)`,
      `List`, `Revoke`, `last_used_at` updated at most once per minute. Scopes: `read`, `write`. `0004_token_ids.sql`
      adds the public id used for lookup.
- [x] **M3.2 CLI** — `homeservices token create --name claude-code --scope write`, `token list`, `token revoke <name>`
      (works before any UI exists).
- [x] **M3.3 Auth middleware** — `/mcp` and `/api/v1`: Bearer token required **unless** the request already has a UI
      session (password mode) or comes from the UI with `HS_AUTH=none` *and* same-origin (`Sec-Fetch-Site: same-origin`).
      Actor in audit = `mcp:<token name>` / `api:<token name>` / `ui`. Read scope + write method → 403.
      Scope is also passed in the context so MCP write tools can refuse read tokens.
- [x] **M3.4 MCP server** — official `github.com/modelcontextprotocol/go-sdk` v1.8.0, Streamable HTTP at `/mcp`,
      stateless + JSON responses. Server instructions explain the data model and "never ask for passwords; logins are
      Vaultwarden links". — `internal/mcp`
- [x] **M3.5 Read tools** — `search`, `list_hosts`, `get_host`, `list_services`, `list_projects`, `get_project`,
      `list_locations`, `list_tags`, `list_access` (method/account/link only), `list_app_links`,
      `suggest_free_port`, `used_by`. Outputs: compact JSON + a one-line summary; lists paginated (`limit`, `cursor`).
- [x] **M3.6 Write tools** (scope `write`) — `create_host`, `update_host` (patch semantics), `delete_host`,
      `add_service`, `update_service`, `remove_service`, `add_ssh_access`, `remove_ssh_access`, `create_project`,
      `update_project`, `delete_project`, `link_project`, `link_project_service`, `add_link`, `add_app_link`,
      `add_project_env`, `add_access`, `connect_service`, `restore`. `readOnlyHint`/`destructiveHint` set.
- [x] **M3.7 Patch semantics in the store** — `PatchHost` changes only given fields; addresses/SSH update by id or
      add new; `location_id: 0` clears. API `PATCH /api/v1/hosts/{id}` uses it. — `inventory/patch.go`
- [x] **M3.8 Resources** — `homeservices://host/{id}`, `homeservices://project/{slug}` and
      `homeservices://inventory` as Markdown summaries; `/inventory.md` plain endpoint (same renderer, read token or
      UI session) for agents without MCP. — `inventory/markdown.go`, `mcp/resources.go`
- [x] **M3.9 Settings › MCP (minimal page)** — `/settings/mcp`: token list (name, scope, created, last used, state),
      *New token* (plaintext shown once + `claude mcp add` command + Vaultwarden hint), *Revoke*. — `internal/web/pages.go`
- [x] **M3.10 Tests** — token verify/revoke/scope; MCP via in-memory transport: list tools, read scope can't write,
      write creates host + audit actor `mcp:test`, resources, `restore`; HTTP MCP 401 without token and end-to-end
      calls with a write token.
- [x] **M3.11 Real-agent check** — Claude Code 2.1.210 connected to the deployed MCP server with the propose
      token and read the inventory correctly (`list_hosts`, `list_services`, `homeservices://inventory`). Its
      print-mode client would not register `propose_changes` (a client-side tool-registration issue, not a server
      error), so the proposal was exercised over the same endpoint with the documented JSON-RPC call; changeset #1
      was reviewed and applied. Recheck the write-tool handshake from the interactive Claude Code UI when convenient.

Risks: MCP SDK API churn → pin the version, wrap it in `internal/mcp` so tools don't import the SDK directly.

---

## M4 — Real pages (server-rendered)

Goal: replace prototype screens with real data, same look. The prototype's `app.css` / `boot.js` / `themes.js` are
reused as-is; `app.js` is split into small modules as pages move over.

- [x] **M4.1 Templating decision** — `html/template` + shared `templates/layout.html` with one `content` template per
      page, parsed at startup from the embedded FS. Recorded in PLAN §13.
- [x] **M4.2 Layout** — server-rendered sidebar (collapsible + remembered), topbar, breadcrumbs, flash toast, mobile
      nav; prototype `app.css`/`boot.js` reused; htmx 2.0.4 vendored in `static/` (no CDN). `static/ui.js` handles
      sidebar/theme/copy/`/` search. — `internal/web/ui.go`, `templates/`
- [x] **M4.3 Settings store** — `internal/settings`: get/all/upsert with audit; sidebar preference is saved
      server-side and rendered on the next request while localStorage keeps the instant toggle. — `internal/settings`
- [x] **M4.4 Home — Tiles** — host, project and service tiles with group filter, numbers strip, remembered
      Details/Site tile-click setting, empty state. Status dots and the attention bar arrive with M5 checks.
- [x] **M4.5 Home — Launcher** — app icons from app links + service URLs, deduped by URL, with a client-side filter.
      (The full ⌘K palette does the server-side search.)
- [x] **M4.6 Hosts list + host page** — list with `?q=` filter, new/edit pages, detail with addresses, SSH logins
      with copy buttons, services (add/remove), projects, access (add) and audit timeline. A cross-host `/services`
      page adds service/host/port/domain/tag/status columns and a text filter.
- [x] **M4.7 Projects list + project page** — grouped list, new/edit pages; detail has env switch (chips), add env,
      add app link, resolved app links, connected services, access (add), hosts/services per env and rendered runbook.
- [x] **M4.8 Forms** — create/edit/delete pages for hosts, projects, services, app links and access entries; add
      env/app link/record/cost; location/group/tag suggestions via `<datalist>`; server-side errors re-render the
      form; delete toast with Undo (restore); local-only redirect guard. htmx drawer swaps are deferred (pages are
      simpler and fully functional); htmx stays vendored for later partials.
- [x] **M4.9 ⌘K palette** — backed by `/api/v1/search`; arrow/enter navigation, Esc, `/` to open; quick actions
      ("New host", "New project", "MCP & tokens"). — `static/ui.js`, `templates/layout.html`
- [x] **M4.10 Icons & favicons** — services fetch their site favicon on save (HTML `<link rel=icon>` then
      `/favicon.ico`), size-capped to 256 KB and image-only, cached in `data/icons/` and served at `/icons/`.
      Private/LAN targets are refused unless enabled; both toggles live in Settings › MCP & tokens. Letter tiles
      remain the fallback. Bundled brand icons are deferred. — `internal/icons`
- [x] **M4.11 Exports** — `~/.ssh/config` download (`/export/ssh-config`) and JSON export button
      (`/export/inventory.json`), both from the dashboard.
- [x] **M4.12 Remove replaced prototype routes** — real pages own `/`, `/hosts`, `/projects`, `/settings/mcp`;
      the rest of the prototype lives under `/design/*` as a preview (shared `app.css`/`boot.js` still at `/assets/`).

Done when: daily flow works without the prototype — add a host, its services, a project with links, find it with ⌘K.

---

## M5 — Checks & scheduler

- [x] **M5.1 Schema** — `0005_checks.sql` (plus `0018_cert_expiry.sql`): checks, results, hourly rollups; partial
      unique index per target/kind; soft delete; cert days/issuer.
- [x] **M5.2 Scheduler** — `internal/checks`: 5 s ticker, worker pool (8 default), reconcile each tick, state machine
      `up → failing → down → recovering → up`, graceful shutdown. (Per-host limit/jitter not needed at this scale.)
- [x] **M5.3 Probes** — TCP connect; HTTP(S) status range/keyword/ignore-TLS; TLS expiry + issuer + SAN match;
      DNS with expected IP; per-check timeouts.
- [x] **M5.4 SSH key-only probe** — no real password is sent: `key_only` / `password_allowed` / `unreachable`,
      updates `ssh_access.status`.
- [x] **M5.5 Auto checks** — reconcile keeps one check per service (TCP/HTTP), SSH login (1 h) and health-checked
      app link; disappearing targets lose their auto check; manual checks untouched.
- [x] **M5.6 Retention** — raw results 30 days, hourly rollups (average latency) 1 year, pruned hourly. `VACUUM`
      waits for a dedicated maintenance job.
- [x] **M5.7 UI** — status dots and 24 h availability bars on tiles/hosts/projects, attention bar, `/problems`,
      "Check now", SSH badges with fix guide.
- [x] **M5.8 MCP** — `check_ssh`, `check_service`, `run_check`, `list_checks`, `list_problems`.
- [x] **M5.9 Tests** — TCP/HTTP/TLS/DNS probes against local listeners, password/key-only in-process SSH servers,
      state machine, reconcile, scheduler, prune.

---

## M6 — Deploy to a Debian LXC

- [x] **M6.1** `deploy/homeservices.service` (user `homeservices`, `StateDirectory`, `ProtectSystem=strict`,
      `NoNewPrivileges`, env file `/etc/homeservices/env`, optional defaults), plus a backup service/timer.
- [x] **M6.2** `deploy/install.sh` (user, dirs, env file, units, health wait) and `update.sh` (backup, migration
      dry-run via `migrate --check`, binary swap, restart, health check, auto-rollback).
- [x] **M6.3** `make release` → `bin/homeservices-linux-amd64` with `git describe` version (`-ldflags -X main.version`),
      shown in `/healthz`, the sidebar footer and `homeservices version`.
- [x] **M6.4** Backups — `homeservices backup` (`VACUUM INTO` → `data/backups/…db.gz`, keep 14) + daily systemd
      timer; restore steps in `deploy/README.md`.
- [x] **M6.5** Installed on the home-lab LXC (`<ct-ip>:8080`), LAN-only, `HS_AUTH=none`; schema 18 of 18, service
      and backup timer enabled, `/healthz` green from the LAN. A `claude-code` propose token was created and saved on
      the host at `/root/homeservices-claude-code.token` (mode 0600) for transfer to Vaultwarden. The auto HTTP check
      for `home-services:8080` was created by the scheduler and is `up`.
- [x] **M6.6** Forgejo Actions: vet + test + build on push; release artifact on `v*` tags.
      — `.forgejo/workflows/ci.yml`

---

## M7 — Plugins, discovery & imports

- [x] **M7.1 Plugin framework** — `internal/integrations`: plugin registry with typed config fields → generated
      settings form (`/settings/integrations`), `Test`, `Sync` returning proposed ops stored per plugin (never direct
      writes), last test/sync/error and op list, JSON API, reset/disconnect. Sync is manual (a scheduler can be added
      later). — `internal/integrations`
- [x] **M7.2 SecretStore** — AES-GCM encrypted `data/secrets.enc` keyed by `HS_MASTER_KEY` (hex or passphrase),
      atomic writes, no secrets in SQLite; falls back to in-memory with a warning when no key is configured.
- [x] **M7.3 Fingerprints** — embedded JSON rules (`discovery/fingerprints.json`) match page titles and TCP banners
      (Proxmox, Caddy, Forgejo/Gitea, Vaultwarden, ntfy, Grafana, Home Assistant, Plex, Jellyfin, Nextcloud, Immich,
      TrueNAS, Synology, UniFi, Pi-hole, SSH, Postgres, Redis), with the port table as fallback. Favicon-hash and
      probe-path rules are intentionally omitted for now.
- [x] **M7.4 Subnet scan** — bounded TCP sweep (≤1024 addresses, 96 workers, 400 ms), banner/HTTP-title grab,
      `discovered_hosts`/`discovered_ports` (`0010`), `/discovery` inbox with register (creates host + services) and
      ignore, JSON API and `homeservices scan -subnet …` CLI. Drift detection still todo.
- [x] **M7.5 Proxmox plugin** — read-only API token; nodes → VMIDs, names, kind, memory/disk/status as proposed host
      ops. IPs are not guessed (agent/config not read); matching happens when proposals are applied.
- [x] **M7.6 Cloudflare plugin** — token verify, zones, nameservers and DNS records → proposed domain/record ops
      (registrar expiry needs the account-level API; still todo).
- [x] **M7.7 Docker plugin** — SSH targets + encrypted private key, `docker ps` → proposed services for published
      ports (protocol guessed from the container port); apply matches hosts by name **or address** and undo removes
      the created services. — `integrations/docker.go`
- [x] **M7.8 Caddy routes** — reads `/config/` from the Caddy admin API, extracts host → upstream dial pairs and
      proposes domain-link ops (wildcards skipped). Read-only; never edits Caddy. — `integrations/caddy.go`
- [x] **M7.9 Suggested plugins page** — the Integrations page suggests Proxmox (8006), Caddy (2019), Cloudflare
      (when domains exist) and Electricity, skipping configured plugins; the same list is the MCP `suggest_plugins`
      tool. — `internal/suggest`

## M8 — Alerts, domains, costs, ops

- [x] **M8.1 Notifier interface** + ntfy + webhook — done for check transitions: `down` and recovery `up`,
      10-minute dedupe, `HS_ALERTS_MIN_DOWN` flap guard, labels resolved to `host/service`. Configured with
      `HS_NTFY_URL` (+`HS_NTFY_TOKEN`) and `HS_WEBHOOK_URL`. SSH logins that start accepting passwords also raise a
      one-off warning. **Maintenance windows** (`/maintenance`, per target/entity/all, one-click “1 h” from Problems)
      silence alerts without stopping checks. A **weekly digest** goes through the same notifiers
      (`HS_DIGEST=off`, `HS_DIGEST_DAYS`). Notification **plugins** (ntfy, Gotify, webhook) add per-event switches
      for down/up, new devices, reminders and digest; discovery scans raise new-device events. Domain renewals and
      TLS certificates alert at 30/7/1 days.
      — `internal/alerts`, `internal/integrations/notify.go`, `internal/checks/maintenance.go`, `internal/digest`
- [x] **M8.2 Domains** — `domains` + `dns_records` (`0007`), CRUD/soft delete/audit, validation (plain name,
      date, currency, price), expiry days, records with matched host. UI list/new/edit/detail plus record
      add/delete and delete-with-undo; JSON API under `/api/v1/domains`. Registrar/account/payment stay free text
      for v1 — Cloudflare/OVH import fills the same columns later. — `internal/inventory/domains.go`
- [x] **M8.3 Costs** — `costs` table with CRUD/soft delete/audit; `/costs` shows monthly/yearly equivalents, one-time
      and next-30-days totals, project/payment/auto-renew columns, add/delete-with-undo and CSV export. Domain
      renewal prices are derived from domains (not duplicated). API under `/api/v1/costs`.
      — `internal/inventory/costs.go`
- [x] **M8.4 Expiry calendar** — `/calendar` lists domain renewals by date plus maintenance windows; alerts fire
      once per domain at 30/7/1/0 days (auto-renew OFF adds a nudge) through the same notifiers, state persisted in
      settings and cleared when a domain is renewed. The weekly digest also lists domains expiring within 30 days.
      TLS checks also record certificate days/issuer, shown on the same calendar and alerted at 30/7/1/0 days.
      — `0018_cert_expiry.sql`, `internal/expiry`
- [x] **M8.5 SSH facts** (read-only `inventory` user) — key-only SSH collector (`HS_SSH_KEY`, `HS_SSH_FACTS_USER`)
      gathers OS, kernel, hostname, uptime, disk %, failed units, reboot flag, sshd password/root flags and key
      fingerprints; shown on the host page and stored in `host_facts`; "Collect" button + API.
      — `internal/facts`
- [x] **M8.6 Power plugin** — Electricity integration (price per kWh, optional Home Assistant token), per-host
      manual watts or live Shelly/Tasmota/HA readings polled every 5 minutes, shown on the host page and as
      Electricity rows on Costs. — `internal/power`, `internal/integrations/power.go`

## M9 — AI changesets

- [x] **M9.1** `changesets` + `changeset_ops` (`0012`); `internal/changesets` creates, lists, applies supported ops
      (host/project/domain/dns_record/service) and records per-op errors; undo soft-deletes what the set created.
      (Ops apply one by one rather than in a single SQL transaction because the store methods own their transactions.)
- [x] **M9.2** Token mode: read / propose / write (`0016_token_scopes.sql`). Propose tokens get an MCP server with
      read tools plus `propose_changes`/`request_connection` and no direct write tools; the REST API also accepts
      `POST /api/v1/changesets` for them. Write tokens keep everything. (Rather than rewriting every write tool to
      silently convert, propose tokens simply don’t see the mutating tools — the review queue stays the only path.)
- [x] **Costs/currency extras** — paid-day FX locking (ECB history + manual rates, bank-charged amount wins),
      currency settings and refresh, electricity rows, per-year view and category charts.
- [x] **M9.3** MCP: `propose_changes`, `list_changesets`, `get_changeset`, `detect` (port/title/banner
      fingerprint), `suggest_plugins`, `request_connection` (proposes a connection op that the changeset applier and
      undo understand).
- [x] **M9.4** Review page `/changesets`: pending ops with data/notes, apply, reject, recent list and undo; pending
      count in the sidebar.
- [x] **M9.5** Plugin `Sync` results can be turned into a changeset with “Propose changes” on the integration card —
      one review pipeline for agents and imports.

## M10 — Open-source release

- [x] **M10.1** `homeservices demo` seeds example.com/acme hosts, services, project, envs, app links, access,
      domain and costs into an empty HS_DATA. — `cmd/homeservices/demo.go`
- [x] **M10.2** UI/API defaults use `192.168.1.0/24` and `192.168.1.x` placeholders; the deploy README uses a
      placeholder address, and the `/design` prototype was scrubbed of the maintainer's domain, subnet and username.
- [x] **M10.3** README, CONTRIBUTING, SECURITY and AGPL-3.0 LICENSE. Issue templates are left to the public
      repo host.
- [x] **M10.4** Multi-stage `Dockerfile` (distroless-ish alpine, non-root, `/data` volume) and `compose.yaml`;
      Proxmox LXC helper script is covered by `deploy/install.sh` for now.
- [x] **M10.5** `internal/themes` validates `themes/*.json` (schema, CSS import/url sanitiser, 4.5:1 contrast);
      `themes/solar.json` example; test runs in CI.
- [x] **M10.6** Published a fresh-history public repo at <https://github.com/dawidof/home-services> (one root
      commit, GitHub noreply author). Environment-specific notes stay in the ignored `.private/PLAN.md` and the
      private deployment repo. GitHub Actions runs `go mod tidy -diff`, vet, the race suite, a build and the CLI
      help; the release workflow uploads the Linux binary on `v*` tags.

---

## What's left

Everything in the plan is implemented, tested, deployed to a Debian LXC and published at
<https://github.com/dawidof/home-services>.

1. Optional polish: favicon-hash/probe-path fingerprints.
2. Recheck the `propose_changes` handshake from the interactive Claude Code UI (the print-mode client did not
   register the write tool; the same call works over raw JSON-RPC).
3. Keep the public tree in sync with the private deployment notes in `.private/`.

### Follow-ups from the 2026-11-16 review

- [x] Backups include `data/secrets.enc` and no longer apply migrations (`db.OpenNoMigrate`).
- [x] `install.sh` generates `HS_MASTER_KEY` alongside the session key.
- [x] Favicon settings counts/refetch use real services (no more `ListHosts` without services).
- [x] Partial plugin syncs surface their warning in the flash and API response; the changeset is marked “(partial)”.
- [x] Renamed the misleading `ratelock_test.go` to `currency_paid_test.go`.
- [x] API discovery scans use the same background runner as the UI (`202`, progress, configured ports, new-device
      alerts) and expose `GET /api/v1/discovery/status`.
- [x] Electricity rows inherit the host's project for the by-project chart; integration “configured” state now
      reflects a saved row instead of comparing against defaults; manual FX rates win over ECB everywhere; an empty
      Proxmox node is no longer reported as a permissions problem.
- [x] Suggested integrations (web + MCP), Docker-over-SSH plugin, `detect` and `request_connection` MCP tools.
- [x] Domain renewals lock the paid-day value like costs (`0017_domain_fx.sql`): bank-charged amount wins,
      otherwise the ECB rate of the paid date; shown on Costs and the domain page.
- [x] TLS certificate expiry recorded by checks (`0018`), shown on `/calendar` and alerted at 30/7/1/0 days.
- [x] Split `web/pages.go` by domain (`pages_home.go`, `pages_hosts.go`, `pages_costs.go`,
      `pages_settings.go`, `pages_discovery.go`, `pages_integrations.go`, …); `pages.go` keeps only the
      page struct, routes and shared shell.
- [x] Paid-day FX locking for domain renewals (`0017`), matching costs.
- [x] In-memory login rate limiting: five failed attempts per IP in five minutes → `429` + `Retry-After`;
      a successful sign-in resets the counter.
- [x] Deployed to the LXC and verified end-to-end: `/healthz`, every core page, token auth (401 without a token),
      MCP initialize/tools, a proposed changeset applied through the REST API, and the auto HTTP check reporting
      `up`. Access/checks/backups are live; the propose token is staged on the host for Vaultwarden.
- [x] `deploy/update.sh` exercised on the LXC: it wrote a fresh DB backup, confirmed schema 18 of 18, swapped the
      binary and came back healthy (`updated: ok 1656071`).
- [x] Fresh-history public repo pushed; GitHub Actions `test` job passed (tidy, vet, race, build, `help`) after
      fixing the `tags` trigger nesting in both workflow copies.


## Open decisions

| Decision | Options | Recommendation | Needed by |
|---|---|---|---|
| Templates | templ vs html/template | html/template | M4.1 |
| Queries | sqlc vs hand-written | hand-written until ~2 k lines | M2.16 |
| License | AGPL-3.0 vs MIT/Apache-2.0 | AGPL-3.0 | M10.3 |
| v1 scope cut | merge link concepts (project links + app links + connections), defer costs/power/themes editor | apply | before M4.7 |
| UI auth | none (LAN) vs password | password if reachable from a VPN/edge, else none | M6.5 |
