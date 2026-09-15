#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
echo 'This is the first schema release. Cross-version upgrades have no supported predecessor yet.' >&2
echo 'For a same-version rebuild, take a backup then rerun install.sh --no-build.' >&2
exit 1
