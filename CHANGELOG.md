# Changelog

## Unreleased

### Added
- **Integrations (31 plugins in total):** Hetzner, DigitalOcean, OVHcloud; Tailscale, Headscale; Prometheus, Netdata, Beszel,
  Glances; TrueNAS, Synology, **OpenMediaVault**, NUT (UPS), Redfish/IPMI; Scrutiny (SMART); UniFi, OPNsense, MikroTik, OpenWrt,
  Pi-hole, AdGuard Home, Nginx Proxy Manager, Traefik.
- Several copies of one plugin (for example two Proxmox servers) and a **Connected** list on the integrations page.
- Host health warnings reported by integrations or SSH facts now appear on Alerts, the bell and the dashboard.
- **Exports:** Homepage `services.yaml`, Ansible inventory, Prometheus file-SD targets and a NetBox IP CSV, in the UI and
  under `/api/v1/export/`.
- `homeservices import file.json` loads an export document from the command line (dry run unless `-apply`).
- Dashboard: **Active / Inactive** filter and a collapsed **Favourites** section.
- Sortable tables (Hosts, Services, Costs, Domains).
- Alerts history can show 24 hours, 7 or 30 days.
- A larger, fictional demo lab (`homeservices demo`), and full documentation with a GitHub Pages site.
- The installer and unit file can serve on port 80 (`CAP_NET_BIND_SERVICE`); `update.sh` follows `HS_ADDR`.

### Changed
- Automatic checks for HTTPS services on LAN addresses ignore certificate errors (self-signed certificates are the norm).
- Favicon fetching covers LAN services (when LAN fetching is enabled) and accepts self-signed certificates there.
- Settings › Scanning shows the editable port list instead of a fixed text.
- The old clickable design prototype and its `/design` route were removed; shared assets moved to `assets/`.

### Fixed
- Discovery progress card stayed visible with stale numbers after a scan finished.
- "Add environment" opened an invisible form on project pages.
- A location appeared once per hidden host in the Hosts filter.
- Flash messages missing on several pages; deleting a login or DNS record could land on the wrong page.
- Incident history only covered about 16 hours of results despite saying 7 days.

## v0.1.2 — 2026-11-17

- Favicon-hash fingerprints for Grafana, Portainer and Nextcloud (MurmurHash3 of MIME base64).
- GitHub releases are now created automatically on `v*` tags with the Linux binary attached.

## v0.1.1 — 2026-11-16

- Discovery probes one API path for title-less services (Grafana, Proxmox, ntfy, Portainer).
- Domain pages show nameservers/DNSSEC/transfer lock and can compare them with live DNS.
- Trusted-proxy-aware client addresses (`HS_TRUSTED_PROXIES`), login rate limiting hardening and
  `staticcheck` in CI.

## v0.1.0 — 2026-11-16

First complete release: a single Go binary with an embedded UI, SQLite, an MCP server and
systemd/Docker packaging.

### Inventory
- Hosts, locations, addresses, SSH logins, services, projects, environments, app links, access
  references, domains/DNS records, costs and power readings.
- Tags and groups as real tables, ranked search, copy buttons, free-port suggestions, CSV/JSON/
  ssh-config exports, soft delete with restore and an audit timeline.
- Every write goes through the store in a transaction with an audit row; secrets never enter SQLite.

### Checks & alerts
- TCP, HTTP(S), TLS (expiry/issuer/SAN), DNS and key-only SSH probes with a 5 s scheduler and a
  `up → failing → down → recovering → up` state machine.
- Auto checks for services, SSH logins, app links and TLS; 24 h availability bars, `/problems`,
  maintenance windows and on-demand runs.
- ntfy, Gotify and webhook notifiers with dedupe, flap guard, new-device alerts, weekly digest and
  domain/certificate reminders at 30/7/1 days.

### AI agents
- MCP over Streamable HTTP with `read`, `propose` and `write` token scopes.
- `propose_changes` and a review queue with apply/reject/undo; a propose-only server exposes no
  direct writes.
- Markdown resources (`homeservices://inventory`, host and project pages) and `/inventory.md`.

### Discovery & integrations
- Bounded TCP-connect subnet scans, banners, page titles, JSON fingerprint rules and a
  register/ignore inbox.
- Read-only plugins for Caddy, Proxmox, Cloudflare, Docker over SSH, electricity sensors, ntfy,
  Gotify and webhooks, all producing reviewable changesets.
- Suggested integrations based on the current inventory, plus an agent `detect` helper.

### Operations
- `install.sh`/`update.sh` with backups, migration dry-run, rollback, systemd units and a daily
  backup timer; multi-stage Docker image and compose file.
- CLI: `migrate`, `backup`, `scan`, `demo`, `token` and `hash-password`; `help` lists everything.
- Password login with argon2id, SameSite cookies, login rate limiting and trusted-proxy-aware
  client addresses.
- AGPL-3.0, CONTRIBUTING and SECURITY docs, demo seed data, theme validation and CI with `go vet`,
  `go mod tidy -diff`, `go test -race ./...` and release builds.

[Unreleased]: https://github.com/Dumhouse/homepage/compare/v0.1.2...HEAD
