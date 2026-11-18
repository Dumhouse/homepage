# Deploying Home Services

Target: a small Debian 13 LXC (for example `192.168.1.101`) running systemd.
Everything ships in one static binary; templates, migrations and htmx are embedded.

## Install

```bash
# build the linux binary (from the repo)
make release

# copy it to the host and run the installer
scp bin/homeservices-linux-amd64 root@192.168.1.101:/tmp/
ssh root@192.168.1.101 'mkdir -p /tmp/hs && cd /tmp/hs'   # copy deploy/ too, or clone the repo there
sudo ./deploy/install.sh ./bin/homeservices-linux-amd64
```

The installer creates the `homeservices` system user, `/var/lib/homeservices`
(state + SQLite), `/etc/homeservices/env` (mode 0600), installs the unit and a
daily backup timer, then waits for `/healthz`.

## Environment (`/etc/homeservices/env`)

| Variable | Default | Meaning |
|---|---|---|
| `HS_ADDR` | `:8080` (installer writes `:80`) | listen address; ports below 1024 work because the unit grants `CAP_NET_BIND_SERVICE` |
| `HS_DATA` | `/var/lib/homeservices` | SQLite, backups, future icon cache |
| `HS_AUTH` | `none` | `none` (trusted LAN) or `password` |
| `HS_PASSWORD_HASH` | — | argon2id hash from `homeservices hash-password` |
| `HS_SESSION_KEY` | random per restart | 32-byte hex HMAC key for cookies |
| `HS_COOKIE_SECURE` | unset | set `1` when served over HTTPS |
| `HS_TRUSTED_PROXIES` | — | proxy IPs/CIDRs allowed to set forwarding headers (needed for per-client login rate limiting behind Caddy) |
| `HS_NTFY_URL` | — | full ntfy topic URL, e.g. `https://ntfy.example.com/homeservices` |
| `HS_NTFY_TOKEN` | — | optional Bearer token for a protected ntfy topic |
| `HS_WEBHOOK_URL` | — | optional JSON webhook for check transitions |
| `HS_ALERTS_MIN_DOWN` | `120` | seconds an outage must last before recovery is announced (flap guard) |
| `HS_DIGEST` | `on` | set `off` to skip the weekly digest |
| `HS_DIGEST_DAYS` | `7` | digest period in days |
| `HS_MASTER_KEY` | — | key for `data/secrets.enc` (integration tokens). Any passphrase or 32-byte hex; without it secrets are in-memory only |
| `HS_SSH_KEY` | — | private key for read-only SSH facts (host page "Collect") |
| `HS_SSH_FACTS_USER` | `inventory` | SSH user for facts; needs read-only commands only |

Favicon fetching is controlled in **Settings › MCP & tokens › Favicons** (stored
in SQLite, not env): on/off, and whether private/LAN addresses may be fetched.
Icons are cached in `/var/lib/homeservices/icons/`.

API/MCP tokens are separate from UI auth and always required for `/mcp`:

```bash
sudo -u homeservices env HS_DATA=/var/lib/homeservices \
  /usr/local/bin/homeservices token create --name claude-code --scope write
```

## Update

```bash
make release
scp bin/homeservices-linux-amd64 root@192.168.1.101:/tmp/
ssh root@192.168.1.101 'sudo /path/to/deploy/update.sh /tmp/homeservices-linux-amd64'
```

`update.sh` backs up the DB (without migrating it), runs `migrate --check`, swaps the binary, restarts
and polls `/healthz`. If the new binary does not come up within 20 s it restores
`/usr/local/bin/homeservices.prev` and restarts.

## Backups

The timer runs `homeservices backup -keep 14` daily at 03:30 UTC (±5 min). Each
run writes `data/backups/homeservices-YYYYmmdd-HHMMSS.db.gz` using SQLite's
online `VACUUM INTO`, so it is safe while the service is running. Integration
tokens in `data/secrets.enc` are written next to it as
`homeservices-*.secrets.enc.gz` (600). Old files are pruned beyond 14.

Manual backup:

```bash
sudo -u homeservices env HS_DATA=/var/lib/homeservices /usr/local/bin/homeservices backup
```

Restore:

```bash
sudo systemctl stop homeservices
cd /var/lib/homeservices
sudo rm -f homeservices.db homeservices.db-wal homeservices.db-shm
sudo -u homeservices sh -c 'gunzip -c backups/homeservices-20260101-033000.db.gz > homeservices.db'
# restore integration tokens too, if that backup has them
if [ -f backups/homeservices-20260101-033000.secrets.enc.gz ]; then
  sudo -u homeservices sh -c 'gunzip -c backups/homeservices-20260101-033000.secrets.enc.gz > secrets.enc'
  sudo chmod 600 secrets.enc
fi
sudo systemctl start homeservices
curl -fsS http://127.0.0.1/healthz
```

`HS_MASTER_KEY` in `/etc/homeservices/env` must match the key used when the
secrets were written. If it is lost, remove `data/secrets.enc` and re-enter the
integration tokens in Settings › Integrations.

## Logs

```bash
journalctl -u homeservices -f
journalctl -u homeservices-backup -n 50
systemctl list-timers homeservices-backup.timer
```

## Checks

The scheduler starts with the service, reconciles auto checks every 5 s and
records results locally. Nothing leaves the host except the probes themselves
(TCP/HTTP/TLS/DNS/SSH) and the configured alert destinations.

Alerts fire when a check enters `down` and when it comes back `up` after a real
outage (shorter than `HS_ALERTS_MIN_DOWN` is treated as a flap and stays quiet).
Repeated identical alerts are deduplicated for 10 minutes.

Use **Maintenance** in the sidebar (or the “1 h” button on Problems) to silence
alerts for a target or everything while checks keep running.

## Off-box backups

Set `HS_BACKUP_COPY_TO=/mnt/nas/backups/homeservices` (a mounted share) and/or
`HS_BACKUP_EXEC='rclone copy $HS_BACKUP_FILES remote:path'` in `/etc/homeservices/env`. Allow a copy
directory in the sandbox with `systemctl edit homeservices-backup` → `[Service] ReadWritePaths=/mnt/nas/backups/homeservices`.
