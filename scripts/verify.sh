#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
[ -f checksums.txt ] || { echo 'No bundle checksum manifest found.' >&2; exit 1; }
sha256sum -c checksums.txt
echo 'Checksums verified. Development bundles are unsigned; verify provenance separately.'
