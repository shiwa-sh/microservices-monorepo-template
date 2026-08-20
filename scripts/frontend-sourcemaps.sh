#!/usr/bin/env bash
# Collect the frontend's browser source maps for a release, and keep them out of
# the served bundle (ADR-0503).
#
#   mise run frontend:sourcemaps -- <release>
#
# Two things have to be true at once, and they pull against each other: a stack
# trace from a browser is unreadable without the map, and a map served to browsers
# hands an attacker the unminified application. So the build emits them, this moves
# them out of the served tree into a release-tagged archive, and symbolication reads
# the archive.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

RELEASE="${1:-${GIT_SHA:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}}"
BUILD_DIR="apps/frontend/.next"
OUT="apps/frontend/sourcemaps-${RELEASE}.tar.zst"

[ -d "$BUILD_DIR" ] || fail "no build at ${BUILD_DIR} — run the production build first"

step "collecting source maps for release ${RELEASE}"

maps=$(find "$BUILD_DIR" -name '*.js.map' -type f | wc -l)
[ "$maps" -gt 0 ] || fail "no .js.map files in ${BUILD_DIR} — is productionBrowserSourceMaps on?"

find "$BUILD_DIR" -name '*.js.map' -type f -print0 | tar --null -T - -c --zstd -f "$OUT"
detail "archived ${maps} maps to ${OUT}"

# Out of the served tree. `standalone` copies `.next/static` into the image, so a
# map left here is a map published on the product origin.
find "$BUILD_DIR" -name '*.js.map' -type f -delete
ok "collected ${maps} source maps and removed them from the build output"

warn "the archive is not stored anywhere durable yet — see E14 in the plan"
