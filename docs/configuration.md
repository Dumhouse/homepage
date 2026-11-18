# Configuration

Home Services is configured with **environment variables** (set once, at start) and **settings pages** (stored in the
database, changeable at any time). Defaults are chosen so that a trusted home network works with no configuration at all.

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `HS_ADDR` | `:8080` | listen address (`:80`, `127.0.0.1:8080`, …). The systemd unit may bind low ports. |
| `HS_DATA` | `./data` | directory for the SQLite file, favicon cache, `secrets.enc` and `backups/` |
| `HS_AUTH` | `none` | `none` (trusted network), `password`, or `proxy` (sign-in by your auth proxy: Authelia, Authentik, oauth2-proxy…) |
| `HS_PASSWORD_HASH` | — | argon2id hash for `HS_AUTH=password` — create it with `homeservices hash-password` |
| `HS_SESSION_KEY` | random per start | 32-byte hex HMAC key for login cookies; set it so sessions survive restarts |
| `HS_COOKIE_SECURE` | — | set `1` when served over HTTPS |
| `HS_TRUSTED_PROXIES` | — | comma-separated proxy IPs / CIDRs allowed to set `X-Forwarded-For` / `CF-Connecting-IP` — and, with `HS_AUTH=proxy`, the user header. Required for proxy mode. |
| `HS_AUTH_PROXY_HEADER` | `Remote-User` | header your auth proxy puts the signed-in user in (`X-Forwarded-User`, `X-authentik-username`, …) |
| `HS_AUTH_PROXY_USERS` | — | optional comma-separated allow-list of users; empty lets in anyone the proxy lets in |
| `HS_MASTER_KEY` | — | passphrase or 32-byte hex key that encrypts `secrets.enc` (integration tokens). Without it secrets are memory-only. |
| `HS_NTFY_URL` / `HS_NTFY_TOKEN` | — | ntfy topic and optional token for alerts (the ntfy *plugin* in the UI is the alternative) |
| `HS_WEBHOOK_URL` | — | JSON webhook for check transitions |
| `HS_ALERTS_MIN_DOWN` | `120` | seconds an outage must last before a *recovered* message (the flap guard) |
| `HS_DIGEST` | `on` | `off` skips the weekly digest |
| `HS_DIGEST_DAYS` | `7` | digest period in days |
| `HS_SSH_KEY` | — | private key for the read-only *Collect facts* over SSH |
| `HS_SSH_FACTS_USER` | `inventory` | SSH user used for facts; needs only read-only commands |
| `HS_BACKUP_COPY_TO` | — | `homeservices backup` also copies to this directory (a NAS mount), keeping the same number of files |
| `HS_BACKUP_EXEC` | — | shell command run after each backup; the new files are in `$HS_BACKUP_FILES` (e.g. `rclone copy $HS_BACKUP_FILES b2:lab-backups`) |
| `HS_UPDATE_CHECK` | — | `on` / `off` forces the daily new-release check (otherwise the Settings › Updates switch, off by default) |

Keep `HS_MASTER_KEY` with your backups: without it the encrypted integration secrets cannot be read and you would
re-enter them.

## Command line

```
homeservices [-addr :8080] [-data ./data]       start the server
homeservices version
homeservices migrate [-check]                    apply (or only check) database migrations
homeservices backup [-keep 14] [-copy-to dir] [-exec cmd]
                                                 online backup of the database and secrets.enc
homeservices import [-apply] [-mode merge|replace] file.json
homeservices scan -subnet 192.168.1.0/24 [-ports 22,80,443]
homeservices demo                                seed a fictional example lab (empty data dir only)
homeservices token create --name <n> [--scope read|propose|write]
homeservices token list | revoke <name>
homeservices hash-password                       prints an argon2id hash for HS_AUTH=password
```

Everything except `scan` and the server accepts `-data` to point at another directory.

## Settings pages

Stored in the database; reachable from **Settings** in the sidebar.

| Page | What you set |
|---|---|
| **Display** | date format, 12/24 h clock, start view (tiles or launcher), what a tile click opens |
| **Currency** | default currency, daily ECB rates, manual rates for currencies the ECB does not publish |
| **Integrations** | plugins, notifiers and the export downloads ([guide](integrations.md)) |
| **MCP & AI access** | create and revoke tokens, copy the connect command |
| **Notifications** | what is configured and which events each notifier sends |
| **Scanning** | subnets, scan schedule, ports to check |
| **Favicons** | fetch service icons on or off, and whether LAN addresses may be fetched |

### Favicons

Icons are fetched once per service and cached in `HS_DATA/icons`. For safety, the fetcher refuses private and LAN
addresses by default, so services reachable only on your LAN have no icon until you turn on *Allow LAN addresses* and
press *Fetch missing*. Public domains work out of the box.

### Themes

Light and dark are built in. A custom theme is a JSON file in `themes/` (colours, font, density, radius, optional CSS).
Themes are validated: only local assets, no `@import`, and text must keep a 4.5 : 1 contrast. See `themes/solar.json`.
