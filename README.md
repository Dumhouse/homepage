# Home Services

A self-hosted inventory for a home lab: hosts, their addresses and SSH logins, services,
projects with environments and app links, logins (as Vaultwarden links), domains, costs,
health checks and alerts — in one Go binary with an embedded UI and an MCP server for AI agents.

```
Go · SQLite · html/template + htmx · one binary · no build step for the UI
```

## What it does

- **Inventory** — hosts, locations, services, projects (groups, envs, app links, runbooks),
  access entries (method/account/Vaultwarden link, never secrets), domains + DNS records, costs
  (multi-currency with paid-day rates and optional live electricity from Home Assistant/Shelly/Tasmota).
- **Checks** — TCP/HTTP/TLS/DNS/SSH-key probes, a 5 s scheduler, state machine
  (`up → failing → down → recovering → up`), 24 h availability bars, `/problems` page,
  maintenance windows that silence alerts.
- **Alerts** — ntfy, Gotify and webhook plugins (per-event switches), dedupe, flap guard,
  SSH password-auth warnings, new-device alerts, weekly digest, domain renewals at 30/7/1 days,
  maintenance windows.
- **AI agents** — MCP over Streamable HTTP, token scopes (read/write), patch semantics,
  Markdown resources, `/inventory.md`, and a **changeset review queue** (`propose_changes` →
  human applies/rejects/undoes) so agents can suggest work without writing directly.
- **Discovery** — bounded subnet scan, banner/HTTP-title fingerprints, an inbox to register or
  ignore new hosts and create services from open ports.
- **Integrations** — read-only plugins with encrypted secrets: Caddy routes, Proxmox VMs/CTs,
  Cloudflare zones/records, Docker containers (all as proposed changesets), plus notification
  plugins and an Electricity plugin for power costs. The page suggests plugins based on what is
  already in the inventory.
- **Ops** — systemd unit, install/update scripts with rollback, backups, CSV/JSON/ssh-config
  exports, nightly retention.

## Quick start

```bash
git clone https://github.com/dawidof/home-services.git
cd home-services

make test        # store, API, MCP, probes, UI
make build
./bin/homeservices
# open http://localhost:8080
```

Or seed the demo inventory first (empty data directory required):

```bash
HS_DATA=./demo-data ./bin/homeservices demo
HS_DATA=./demo-data ./bin/homeservices
```

Data lives in `./data` by default (`HS_DATA` to change it). The SQLite file, favicon cache,
secrets and backups all stay in that directory.

## Connect Claude Code / another agent

```bash
./bin/homeservices token create --name claude-code --scope propose   # or write
# store the token in Vaultwarden, then:
claude mcp add --transport http homeservices http://localhost:8080/mcp \
  --header "Authorization: Bearer hs_…"
```

- `read` tokens can search and read everything.
- `propose` tokens can read and queue changesets (no direct writes); a human applies them.
- `write` tokens can create/update/delete directly and propose changesets.
- `/api/v1` and `/mcp` always require a token (unless the request carries a UI session or is
  same-origin with `HS_AUTH=none`).
- Without MCP, agents can fetch `/inventory.md` with the same token.

Agents never get passwords. Access entries point at Vaultwarden items.

## Deploy

A Debian/Ubuntu LXC or VM:

```bash
make release
sudo ./deploy/install.sh ./bin/homeservices-linux-amd64
```

The installer creates the `homeservices` user, `/var/lib/homeservices`, `/etc/homeservices/env`,
the systemd unit and a daily backup timer, then waits for `/healthz`. Updates use
`deploy/update.sh` (backup → migration dry-run → swap → health check → auto-rollback).
See [`deploy/README.md`](deploy/README.md) for backups, restore and the environment table.

### Docker

```bash
docker compose up -d
```

The image runs as a non-root user with `/data` as the volume.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `HS_ADDR` | `:8080` | listen address |
| `HS_DATA` | `./data` | SQLite, icons, secrets, backups |
| `HS_AUTH` | `none` | `none` (trusted LAN) or `password` |
| `HS_PASSWORD_HASH` | — | argon2id hash (`homeservices hash-password`) |
| `HS_SESSION_KEY` | random | HMAC key for cookies |
| `HS_COOKIE_SECURE` | — | `1` behind HTTPS |
| `HS_NTFY_URL` / `HS_NTFY_TOKEN` | — | ntfy topic and optional token |
| `HS_WEBHOOK_URL` | — | JSON webhook for alerts |
| `HS_ALERTS_MIN_DOWN` | `120` | flap guard in seconds |
| `HS_DIGEST` / `HS_DIGEST_DAYS` | on / 7 | weekly digest |
| `HS_MASTER_KEY` | — | key for `data/secrets.enc` (integration tokens) |
| `HS_SSH_KEY` / `HS_SSH_FACTS_USER` | — / `inventory` | read-only SSH facts |

CLI: `token`, `migrate [-check]`, `backup` (DB + `secrets.enc`), `scan -subnet`, `demo`,
`version`, `hash-password`.

## Layout

```
cmd/homeservices      flags + CLI (token, migrate, backup, scan, demo)
internal/inventory    the only writer: validation, soft delete, audit, search, exports
internal/checks       scheduler, probes, auto checks, retention
internal/alerts       ntfy/webhook notifiers + rules
internal/digest       weekly summary
internal/expiry       domain renewal alerts
internal/integrations plugin registry, SecretStore, Caddy/Proxmox/Cloudflare
internal/discovery    subnet scanner + fingerprints
internal/facts        read-only SSH facts
internal/changesets   propose → review → apply/undo
internal/mcp          MCP tools and resources
internal/web          chi router, JSON API, server-rendered UI
templates/ static/    embedded templates, htmx, ui.js (no CDN)
deploy/               systemd units, install/update scripts
design/               original clickable prototype (served under /design as a preview)
```

Ground rules: every write goes through `inventory.Store` in a transaction with an audit row;
soft delete everywhere; migrations are append-only; no secrets in SQLite.

## Status

The full plan is implemented and running on a Debian LXC: inventory, MCP with read/propose/write
scopes, every core page, checks and alerts, discovery and integrations, changesets and the
open-source packaging. Only the fresh-history public repository is left; see
[`IMPLEMENTATION.md`](IMPLEMENTATION.md) for the task list and `PLAN.md` for the design rationale.

`homeservices help` lists every command; `migrate -check`, `backup`, `scan`, `demo` and
`hash-password` are safe to run before the server starts.

## License

AGPL-3.0. See [LICENSE](LICENSE).
