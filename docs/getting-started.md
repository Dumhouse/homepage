# Getting started

This page takes you from nothing to a useful inventory in about ten minutes.

## 1. Install

Pick one. All three give you the same app.

### Docker

```bash
git clone https://github.com/dawidof/home-services.git
cd home-services
docker compose up -d
```

Open <http://localhost:8080>. Data lives in `./data` (a SQLite file, icon cache, backups and
the encrypted secrets file). Set `HS_PORT` to publish a different port.

### Linux server or LXC (systemd)

```bash
make release                                   # builds bin/homeservices-linux-amd64
scp -r bin/homeservices-linux-amd64 deploy root@your-host:/tmp/
ssh root@your-host 'cd /tmp && ./deploy/install.sh ./homeservices-linux-amd64'
```

The installer creates a `homeservices` user, a state directory, an environment file, the systemd unit
and a daily backup timer, then waits for the health check. It listens on port **80** by default
(`HS_PORT=8080 ./deploy/install.sh …` to change it). Prebuilt binaries are attached to every
[GitHub release](https://github.com/dawidof/home-services/releases).

### From source

```bash
make test
make build
./bin/homeservices
```

You need Go (the version in `go.mod`). There is no frontend build.

## 2. Look around with demo data (optional)

```bash
HS_DATA=./demo-data ./bin/homeservices demo
HS_DATA=./demo-data ./bin/homeservices
```

The demo is a fictional lab on the reserved `192.0.2.0/24` network. Its checks cannot reach anything,
so most tiles show as unreachable — that is expected. Delete `./demo-data` when you are done.

## 3. Add what you have

You can start from any of these; they mix freely.

| Way | Good for | How |
|---|---|---|
| **By hand** | a handful of important things | **Add → New host**, then *Add service* on the host page |
| **Discovery** | everything on your LAN | **Discovery → Add subnet** (e.g. `192.168.1.0/24`), scan, then *Register* what you recognise |
| **Integrations** | what already lives in another tool | **Settings → Integrations** → pick Proxmox, Caddy, UniFi… → *Test* → *Sync now* |
| **An AI agent** | documenting from repos and configs | connect MCP, then ask it to inventory and *propose* changes ([guide](ai-agents.md)) |

Imports never write straight into your inventory. A sync creates a **changeset** on the *AI changes*
page; you read it, apply it, and can undo it later.

## 4. Let it watch

Any service with a port or a public domain gets an automatic health check (HTTP for web services,
TCP otherwise). SSH logins are checked for key-only access. You will see:

- coloured dots and a 24 h availability bar on every tile,
- the **Alerts** page for what is down now and what went down before,
- a bell with a count in the top bar.

Use **Maintenance** to silence alerts while you work on something.

## 5. Get told when something breaks

Add a notifier under **Settings → Integrations → Notify**: [ntfy](https://ntfy.sh), Gotify or a
generic webhook. Each has switches for *goes down*, *comes back*, *new device on the network* and
*expiry reminders / weekly digest*. Press **Test** to send yourself a message.

## 6. Connect an AI agent (optional)

```bash
./bin/homeservices token create --name claude-code --scope propose
claude mcp add --transport http homeservices http://localhost:8080/mcp \
  --header "Authorization: Bearer hs_…"
```

Then ask: *"Look at this repo's docker-compose and propose the services to add."* Details and the
list of tools are in [AI agents](ai-agents.md).

## 7. Protect it

The default is **no login**, because the app is designed for a trusted network. If the port is
reachable from anywhere you do not trust:

```bash
./bin/homeservices hash-password                  # prints an argon2id hash
HS_AUTH=password HS_PASSWORD_HASH='…' ./bin/homeservices
```

Behind HTTPS also set `HS_COOKIE_SECURE=1`. API and MCP tokens are always required, whatever
`HS_AUTH` says.

## Where next

- [Concepts](concepts.md) — what a host, service and project mean here
- [Integrations](integrations.md) — set up Proxmox, Caddy, MikroTik, OpenMediaVault…
- [Deployment](deployment.md) — reverse proxy, backups, updates
