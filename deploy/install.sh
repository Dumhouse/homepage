#!/usr/bin/env bash
# Install Home Services on a Debian/Ubuntu host (e.g. a Proxmox LXC).
# Usage: sudo ./deploy/install.sh ./bin/homeservices-linux-amd64
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root (sudo)" >&2
  exit 1
fi
BIN="${1:-./bin/homeservices-linux-amd64}"
[ -f "$BIN" ] || { echo "binary not found: $BIN" >&2; exit 1; }

if ! id homeservices >/dev/null 2>&1; then
  useradd --system --home /var/lib/homeservices --shell /usr/sbin/nologin homeservices
fi
install -d -o homeservices -g homeservices -m 0750 /var/lib/homeservices
install -d -m 0750 /etc/homeservices
install -m 0755 "$BIN" /usr/local/bin/homeservices

if [ ! -f /etc/homeservices/env ]; then
  key="$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  master="$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  cat >/etc/homeservices/env <<EOF
# Home Services environment. Read by systemd before dropping to homeservices.
HS_ADDR=:${HS_PORT:-80}
HS_DATA=/var/lib/homeservices
# none = trusted LAN; password = require a login (set HS_PASSWORD_HASH).
HS_AUTH=none
# HS_PASSWORD_HASH=argon2id\$...
HS_SESSION_KEY=$key
# HS_COOKIE_SECURE=1
# Encrypts integration tokens in data/secrets.enc. Keep it with the backups —
# without it the tokens cannot be read (re-enter the integrations instead).
HS_MASTER_KEY=$master
# Optional: read-only key for the host page "Collect SSH facts".
# HS_SSH_KEY=/etc/homeservices/id_ed25519
# HS_SSH_FACTS_USER=inventory
EOF
  chmod 0600 /etc/homeservices/env
  echo "wrote /etc/homeservices/env (back it up somewhere safe)"
fi

install -m 0644 "$(dirname "$0")/homeservices.service" /etc/systemd/system/homeservices.service
install -m 0644 "$(dirname "$0")/homeservices-backup.service" /etc/systemd/system/homeservices-backup.service
install -m 0644 "$(dirname "$0")/homeservices-backup.timer" /etc/systemd/system/homeservices-backup.timer
systemctl daemon-reload
systemctl enable --now homeservices.service
systemctl enable --now homeservices-backup.timer

echo "waiting for /healthz…"
for _ in $(seq 1 15); do
  if curl -fsS http://127.0.0.1:${HS_PORT:-80}/healthz >/dev/null 2>&1; then
    curl -fsS http://127.0.0.1:${HS_PORT:-80}/healthz
    echo "installed: $(/usr/local/bin/homeservices version)"
    exit 0
  fi
  sleep 1
done
echo "service did not become healthy; check: journalctl -u homeservices -n 80" >&2
exit 1
