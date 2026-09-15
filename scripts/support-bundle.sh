#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
umask 077
mkdir -p dist/support
stamp=$(date -u +%Y%m%dT%H%M%SZ)
# Deliberately omit environment, raw logs, database rows, source files and credentials.
podman version --format '{{.Client.Version}}' > "dist/support/version-$stamp.txt"
podman compose --env-file .env -f deploy/compose/compose.json ps > "dist/support/services-$stamp.txt"
curl -fsS http://localhost:8088/healthz > "dist/support/health-$stamp.json" || true
tar -czf "dist/support-$stamp.tar.gz" -C dist/support "version-$stamp.txt" "services-$stamp.txt" "health-$stamp.json"
echo "Created dist/support-$stamp.tar.gz"
