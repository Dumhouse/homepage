# FAQ & troubleshooting

## General

**Is this a monitoring tool or an inventory?**
Both, on purpose. A pure inventory goes stale; a pure monitor does not know what things *are*. Home Services keeps the
inventory next to the checks, so it shows what is documented *and* whether it really answers. It is not trying to replace
Prometheus or Grafana for metrics.

**How is it different from NetBox, Homepage or Uptime Kuma?**
NetBox is a heavyweight source of truth for networks; Homepage is a start page; Uptime Kuma is a status monitor. Home Services
sits between them for a home lab: light enough to run in a small container, with projects, logins-as-links, domains and costs,
health checks, and an MCP server. It can **export** to Homepage, Ansible, Prometheus and NetBox, so it complements them.

**Where is my data?**
In `HS_DATA`: `homeservices.db` (SQLite), `icons/`, `secrets.enc` and `backups/`. Back that directory up.

**Does it phone home?**
No telemetry. The only outbound traffic is what you configure: probes, alerts, integrations, and optional public lookups
(RDAP for domains, ECB exchange rates, favicons) that each have a switch.

## Hosts, services and checks

**Why do I have both a Hosts and a Services page?**
Hosts are machines; services are what runs on them. Services is a flat list across all machines so you can find "Grafana"
without remembering where it lives. See [Concepts](concepts.md).

**A service shows "down" but I know it is up.**
The check runs from the machine Home Services is on. Open the tile and read the last error: *no route to host* or
*i/o timeout* means that machine cannot reach it (firewall, VLAN, wrong address); a TLS error on HTTPS usually means a
self-signed certificate; *connection refused* means nothing listens on that port. Fix the address/port on the service, or
remove the check from services that do not need one.

**Self-signed certificates fail.**
Automatic checks for HTTPS services on LAN addresses ignore certificate errors. For public domains the certificate must be
valid — that is the point of the public check.

**The progress bar on Discovery is stuck.**
Fixed in recent versions (the hidden bar was being shown). Update; if a scan really is running you can leave the page, it
continues in the background.

**Scanning finds nothing.**
The scanner needs a TCP connection, so it only sees hosts that answer on the configured ports. Add the ports you care about
under *Settings → Scanning*. The subnet must be reachable from where the app runs, up to a /22 each.

**I do not see icons for my LAN services.**
Turn on *Settings → Favicons → Allow LAN addresses* and press *Fetch missing*. They are off by default for safety.

## Integrations

**Sync says "refused the credentials" or "HTTP 403".**
Check the user or token and its permissions — most plugins only need read access. For an ntfy server that requires login, create
a token and paste it.

**Can I have two Proxmox servers?**
Yes. Open the plugin and press *Add another Proxmox*. Each copy has its own settings and secrets.

**Why did a sync add nothing?**
Everything it found already exists. Syncs only propose what is new; unchanged items are left out.

## Security

**Is it safe to expose to the internet?**
Not as is. Put it behind a VPN (Tailscale, WireGuard) or a reverse proxy with authentication, and set `HS_AUTH=password`. API and
MCP tokens are always required regardless. See [SECURITY.md](https://github.com/dawidof/home-services/blob/main/SECURITY.md).

**Where do passwords go?**
Nowhere — the app stores only a link to your password manager. Integration tokens are the exception and are AES-GCM encrypted in
`secrets.enc`; set `HS_MASTER_KEY` so they survive restarts.

**What can an AI agent do?**
Only what its token's scope allows: `read`, `propose` (queue changes you must apply) or `write`. It never receives a password
or an integration secret. See [AI agents](ai-agents.md).

## Operations

**How do I update?** `deploy/update.sh` (backup, migration check, swap, health check, automatic rollback) or
`docker compose up -d --build`. See [Deployment](deployment.md).

**How do I move to another machine?** Copy the `HS_DATA` directory and `HS_MASTER_KEY`, or use
`homeservices backup` then restore on the new machine.

**I lost `HS_MASTER_KEY`.** Delete `data/secrets.enc` and re-enter your integration tokens; the inventory itself is unaffected.
