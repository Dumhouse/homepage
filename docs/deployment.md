# Deployment

Home Services is a single static binary plus a data directory, so there is not much to deploy. This page covers the
three usual setups, putting it behind a reverse proxy, updating, and backups.

> **Decide first:** who can reach it? The default has no login. On a trusted LAN that is fine. Anywhere else, set
> `HS_AUTH=password`, and prefer running it behind your VPN or a reverse proxy with its own authentication.

## systemd on a Debian/Ubuntu LXC or VM (recommended)

Download `homeservices-linux-amd64` (or `-arm64` for a Raspberry Pi 4/5, `-armv7` for older Pis) from the
[latest release](https://github.com/Dumhouse/homepage/releases/latest) — check it against `SHA256SUMS` — or build it:

```bash
make release                                           # bin/homeservices-linux-{amd64,arm64,armv7}
scp -r bin/homeservices-linux-amd64 deploy root@HOST:/tmp/
ssh root@HOST 'cd /tmp && ./deploy/install.sh ./homeservices-linux-amd64'
```

The installer:

- creates the `homeservices` system user and `/var/lib/homeservices` (state and SQLite),
- writes `/etc/homeservices/env` (mode 0600) with fresh `HS_SESSION_KEY` and `HS_MASTER_KEY`,
- installs the hardened systemd unit and a **daily backup timer** (03:30 UTC),
- starts the service and waits for `/healthz`.

It listens on port **80** (the unit grants `CAP_NET_BIND_SERVICE`). Pick another port with `HS_PORT=8080 ./deploy/install.sh …`
or edit `HS_ADDR` in `/etc/homeservices/env` and restart.

Logs: `journalctl -u homeservices -f`.

## Docker

```bash
docker compose up -d                 # pulls ghcr.io/dumhouse/homepage:latest (amd64, arm64, arm/v7)
docker compose up -d --build         # or build it from this checkout
```

Pin a version with `HS_VERSION=0.2.0` in `.env`. The image runs as a non-root user; `/data` is the volume (mapped to `./data`). Set `HS_PORT` to change the published
port, and pass `HS_*` variables through `compose.yaml` or an `.env` file. The container has a health check on `/healthz`.

## Behind a reverse proxy

Caddy:

```caddyfile
inventory.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

nginx:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
}
```

Then set `HS_COOKIE_SECURE=1` (HTTPS only) and `HS_TRUSTED_PROXIES=<proxy ip>` so login rate-limiting sees the real client
address. The UI uses plain requests (htmx and polling), so no WebSocket or streaming configuration is needed.

### Sign-in by your auth proxy (Authelia, Authentik, oauth2-proxy)

If you already have single sign-on in front of your services, let it do the login:

```bash
HS_AUTH=proxy
HS_TRUSTED_PROXIES=192.168.1.10        # the reverse proxy's address — required
HS_AUTH_PROXY_HEADER=Remote-User       # the default; Authentik uses X-authentik-username
HS_AUTH_PROXY_USERS=alice,bob          # optional allow-list
```

Caddy with Authelia:

```caddyfile
inventory.example.com {
    forward_auth authelia:9091 {
        uri /api/authz/forward-auth
        copy_headers Remote-User Remote-Groups
    }
    reverse_proxy 192.168.1.20:80
}
```

Home Services believes the header **only** from addresses in `HS_TRUSTED_PROXIES`; any other client gets *403*, so nobody
can sign in by sending the header themselves. Make sure the app's port is not reachable around the proxy (bind it to the
proxy's network or firewall it). The signed-in user is recorded in the audit log as `ui:<user>`. `/mcp` and `/api/v1`
still need a token, so agents keep working without the proxy.

## Updating

```bash
curl -fLO https://github.com/Dumhouse/homepage/releases/latest/download/homeservices-linux-amd64
scp homeservices-linux-amd64 root@HOST:/tmp/
ssh root@HOST '/path/to/deploy/update.sh /tmp/homeservices-linux-amd64'
```

Want to hear about new versions? Turn on **Settings › Updates**: once a day it asks GitHub for the latest release
(nothing about your install is sent) and shows a note in the sidebar when one is out. It is off by default.

`update.sh` does, in order: **backup** the database → **dry-run** the migrations → swap the binary → restart →
poll `/healthz`. If the new version does not come up within 20 seconds it **rolls back** to the previous binary
automatically. It also picks up changes to the systemd unit shipped with the release, and reads the port from the env
file, so it works whichever port you chose.

Docker: `docker compose pull && docker compose up -d` (or `git pull && docker compose up -d --build` from source). Migrations are applied automatically on start and are append-only.

## Backups and restore

The timer runs `homeservices backup -keep 14` daily. It uses SQLite's online `VACUUM INTO`, so it is safe while the app
runs, and writes `backups/homeservices-YYYYmmdd-HHMMSS.db.gz` plus the encrypted `secrets.enc` next to it.

```bash
# manual backup
sudo -u homeservices env HS_DATA=/var/lib/homeservices /usr/local/bin/homeservices backup
```

Restore:

```bash
sudo systemctl stop homeservices
cd /var/lib/homeservices
sudo rm -f homeservices.db homeservices.db-wal homeservices.db-shm
sudo -u homeservices sh -c 'gunzip -c backups/homeservices-20260101-033000.db.gz > homeservices.db'
sudo systemctl start homeservices
curl -fsS http://127.0.0.1/healthz
```

Restore `secrets.enc` too if the backup has one, and keep the same `HS_MASTER_KEY`.

### Off-box copies

A backup on the same disk is not a backup. Two built-in ways to get it off the machine, both applied by the daily timer
through `/etc/homeservices/env`:

```bash
# 1. a mounted NAS share (NFS/SMB) — files are copied and pruned to the same -keep count
HS_BACKUP_COPY_TO=/mnt/nas/backups/homeservices

# 2. any command, e.g. rclone to S3, B2 or another server; new files are in $HS_BACKUP_FILES
HS_BACKUP_EXEC='rclone copy $HS_BACKUP_FILES b2:lab-backups/homeservices'
```

The systemd unit is sandboxed, so allow the copy target with a drop-in:

```bash
sudo systemctl edit homeservices-backup      # add:
# [Service]
# ReadWritePaths=/mnt/nas/backups/homeservices
```

A failed copy or command makes the backup job fail (visible in `systemctl status homeservices-backup`), while the local
backup is kept.

## Monitoring the monitor

- `GET /healthz` returns `ok <version>` — point your uptime tool at it.
- Add Home Services to its own inventory, and a second instance (or any external pinger) to watch it.
- Checks run from wherever the app runs, so deploy it on a machine that can reach what you want to monitor.

More detail on every variable is in [Configuration](configuration.md); the full ops reference lives in
[`deploy/README.md`](https://github.com/Dumhouse/homepage/blob/main/deploy/README.md).
