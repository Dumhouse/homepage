# Deployment

Home Services is a single static binary plus a data directory, so there is not much to deploy. This page covers the
three usual setups, putting it behind a reverse proxy, updating, and backups.

> **Decide first:** who can reach it? The default has no login. On a trusted LAN that is fine. Anywhere else, set
> `HS_AUTH=password`, and prefer running it behind your VPN or a reverse proxy with its own authentication.

## systemd on a Debian/Ubuntu LXC or VM (recommended)

```bash
make release                                           # bin/homeservices-linux-amd64
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
docker compose up -d
```

The image runs as a non-root user; `/data` is the volume (mapped to `./data`). Set `HS_PORT` to change the published
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

## Updating

```bash
make release
scp bin/homeservices-linux-amd64 root@HOST:/tmp/
ssh root@HOST '/path/to/deploy/update.sh /tmp/homeservices-linux-amd64'
```

`update.sh` does, in order: **backup** the database → **dry-run** the migrations → swap the binary → restart →
poll `/healthz`. If the new version does not come up within 20 seconds it **rolls back** to the previous binary
automatically. It also picks up changes to the systemd unit shipped with the release, and reads the port from the env
file, so it works whichever port you chose.

Docker: `git pull && docker compose up -d --build`. Migrations are applied automatically on start and are append-only.

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

Restore `secrets.enc` too if the backup has one, and keep the same `HS_MASTER_KEY`. Copy backups off the machine — a backup
on the same disk is not a backup.

## Monitoring the monitor

- `GET /healthz` returns `ok <version>` — point your uptime tool at it.
- Add Home Services to its own inventory, and a second instance (or any external pinger) to watch it.
- Checks run from wherever the app runs, so deploy it on a machine that can reach what you want to monitor.

More detail on every variable is in [Configuration](configuration.md); the full ops reference lives in
[`deploy/README.md`](https://github.com/dawidof/home-services/blob/main/deploy/README.md).
