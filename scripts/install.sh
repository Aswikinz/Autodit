#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
mode=${1:-online}
if [ "$mode" = online ] && [ ! -f Containerfile ] && [ -f checksums.txt ]; then mode=--offline; fi
command -v podman >/dev/null || { echo 'Podman 4.4+ is required.' >&2; exit 1; }
podman info >/dev/null
podman compose version
if [ "$mode" = '--preflight' ]; then
  printf 'Architecture: '; uname -m
  df -h .
  if [ "$(id -u)" != 0 ]; then
    grep "^$(id -un):" /etc/subuid >/dev/null || { echo 'Missing subordinate UID range'; exit 1; }
    grep "^$(id -un):" /etc/subgid >/dev/null || { echo 'Missing subordinate GID range'; exit 1; }
    if command -v loginctl >/dev/null; then loginctl show-user "$(id -un)" -p Linger; fi
  fi
  echo 'Preflight finished. No installation changes made.'; exit 0
fi
if [ -f .env ] && ! grep -qx 'AUTODIT_VERSION=0.1.0' .env; then echo 'Use the upgrade runbook for an existing version.' >&2; exit 1; fi
umask 077
mkdir -p secrets inbox
chmod 755 inbox
for name in db_password app_password demo_token oidc_secret admin_password; do
  if [ ! -f "secrets/$name" ]; then head -c 36 /dev/urandom | base64 > "secrets/$name"; fi
done
for name in db_password app_password demo_token oidc_secret admin_password; do
  podman secret exists "autodit_$name" || podman secret create "autodit_$name" "secrets/$name"
done
[ -f .env ] || cp .env.example .env
port=${AUTODIT_PORT:-$(sed -n 's/^AUTODIT_PORT=//p' .env | tr -d '\r')}
port=${port:-8088}
case "$port" in *[!0-9]*|'') echo 'Invalid AUTODIT_PORT'; exit 1;; esac
[ "$port" -ge 1 ] && [ "$port" -le 65535 ] || { echo 'Invalid AUTODIT_PORT'; exit 1; }
public_url=${AUTODIT_PUBLIC_URL:-$(sed -n 's/^AUTODIT_PUBLIC_URL=//p' .env | tr -d '\r')}
public_url=${public_url:-http://localhost:$port}
if [ "$mode" = '--offline' ]; then
  sh scripts/verify.sh
  for image in dist/images/*.tar; do podman load -i "$image"; done
elif [ "$mode" != '--no-build' ]; then
  podman build -t localhost/autodit:0.1.0 -f Containerfile .
  podman build -t localhost/autodit-proxy:0.1.0 -f Containerfile.proxy .
fi
compose() { podman compose --env-file .env -f deploy/compose/compose.json "$@"; }
compose up -d postgres
compose --profile setup run --rm migrate
compose --profile setup run --rm bootstrap
compose up -d --force-recreate api worker proxy
attempt=0
until curl --max-time 3 -fsS "http://localhost:$port/healthz" >/dev/null; do
  attempt=$((attempt+1)); [ "$attempt" -lt 60 ] || { echo 'Health check timed out.'; exit 1; }; sleep 2
done
echo "Autodit is ready at $public_url"
echo 'Default username: admin. Read secrets/admin_password for the initial password.'
