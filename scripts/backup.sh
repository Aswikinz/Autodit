#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
umask 077
mkdir -p dist/backups
stamp=$(date -u +%Y%m%dT%H%M%SZ)
compose() { podman compose --env-file .env -f deploy/compose/compose.json "$@"; }
# Quiesce both writers so the database and snapshot volume share one boundary.
compose stop -t 120 api worker
trap 'compose start api worker' EXIT HUP INT TERM
compose exec -T postgres pg_dump -U autodit_owner -d autodit -Fc > "dist/backups/database-$stamp.dump"
podman volume export autodit_snapshots --output "dist/backups/snapshots-$stamp.tar"
sha256sum "dist/backups/database-$stamp.dump" "dist/backups/snapshots-$stamp.tar" > "dist/backups/checksums-$stamp.txt"
echo "Backup saved under dist/backups with timestamp $stamp. Copy it and the separate secrets to protected offline storage."
