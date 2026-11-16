#!/usr/bin/env bash
# Update a running Home Services install with automatic rollback.
# Usage: sudo ./deploy/update.sh ./bin/homeservices-linux-amd64
#
# Steps: backup DB → migration dry-run → swap binary → restart → health check →
# roll back to the previous binary if /healthz does not come back.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root (sudo)" >&2
  exit 1
fi
BIN="${1:-./bin/homeservices-linux-amd64}"
[ -f "$BIN" ] || { echo "binary not found: $BIN" >&2; exit 1; }
HEALTH="${HS_HEALTH_URL:-http://127.0.0.1:8080/healthz}"
TARGET=/usr/local/bin/homeservices

run_as_app() {
  if command -v runuser >/dev/null 2>&1; then
    runuser -u homeservices -- env HS_DATA=/var/lib/homeservices "$@"
  else
    sudo -u homeservices env HS_DATA=/var/lib/homeservices "$@"
  fi
}

echo "1/5 backup"
run_as_app "$TARGET" backup -keep 14 || echo "warning: backup failed, continuing"

echo "2/5 migration dry-run"
set +e
run_as_app "$TARGET" migrate --check
pending=$?
set -e
if [ "$pending" -eq 2 ]; then
  echo "    migrations are pending; they apply automatically on service start"
elif [ "$pending" -ne 0 ]; then
  echo "migration check failed; aborting" >&2
  exit 1
else
  echo "    schema already current"
fi

echo "3/5 install new binary"
install -m 0755 "$BIN" /usr/local/bin/homeservices.new
cp -f "$TARGET" /usr/local/bin/homeservices.prev
mv -f /usr/local/bin/homeservices.new "$TARGET"

echo "4/5 restart"
systemctl restart homeservices.service

echo "5/5 health check"
for _ in $(seq 1 20); do
  if curl -fsS "$HEALTH" >/dev/null 2>&1; then
    echo "updated: $(curl -fsS "$HEALTH")"
    exit 0
  fi
  sleep 1
done

echo "health check failed — rolling back" >&2
mv -f /usr/local/bin/homeservices.prev "$TARGET"
systemctl restart homeservices.service
for _ in $(seq 1 15); do
  if curl -fsS "$HEALTH" >/dev/null 2>&1; then
    echo "rolled back to the previous binary" >&2
    exit 1
  fi
  sleep 1
done
echo "rollback also unhealthy; check: journalctl -u homeservices -n 120" >&2
exit 1
