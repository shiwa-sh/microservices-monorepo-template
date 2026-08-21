#!/usr/bin/env bash
# Every TypeScript workspace member is copied into the frontend image build.
#
#   mise run lint:workspace-copies
#
# `bun install` resolves the workspace list BEFORE installing anything, so a member
# named in the root package.json and absent from the build context fails the install
# outright with "Workspace not found". The frontend Dockerfile's deps stage already
# says a new member needs a line — and two were added without one, which is why this
# is a gate now rather than a comment.
#
# The failure is expensive out of proportion to its size: it does not appear in any
# lint or test, only when the image is built, which on the full tier is minutes into
# a cluster bring-up.
#
# GLOB members (`libs/ts/sdks/*`) are checked differently, not exempted. They used
# to be, on the theory that the SDKs are generated rather than committed — but they
# ARE committed, like every generated artefact here, so bun resolves each one and
# fails the install without its manifest. A glob cannot be enumerated in a COPY
# without reintroducing the drift this gate exists to catch, so what is required is
# that the glob's PARENT directory is copied wholesale.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MANIFEST="package.json"
DOCKERFILE="apps/frontend/Dockerfile"

[ -f "$MANIFEST" ] || fail "$MANIFEST not found"
[ -f "$DOCKERFILE" ] || fail "$DOCKERFILE not found"

all_members="$(yq -r '.workspaces[]' -p json "$MANIFEST" 2>/dev/null || true)"
members="$(grep -v '\*' <<<"$all_members" || true)"
globs="$(grep '\*' <<<"$all_members" || true)"
[ -n "$members" ] || fail "no explicit workspace members parsed from ${MANIFEST}"

rc=0
count=0
while IFS= read -r member; do
  [ -n "$member" ] || continue
  count=$((count + 1))
  if ! grep -qF "COPY ${member}/package.json" "$DOCKERFILE"; then
    warn "${member} is a workspace member but ${DOCKERFILE} never copies its package.json"
    rc=1
  fi
done <<<"$members"

while IFS= read -r glob; do
  [ -n "$glob" ] || continue
  count=$((count + 1))
  parent="${glob%/*}"
  if ! grep -qE "^COPY ${parent}/? ${parent}/?\$" "$DOCKERFILE"; then
    warn "${glob} matches workspace members but ${DOCKERFILE} never copies ${parent}/"
    rc=1
  fi
done <<<"$globs"

[ "$count" -gt 0 ] || fail "no members checked — the gate would pass vacuously"

if [ "$rc" -eq 0 ]; then
  ok "every workspace member is copied into the frontend build (${count} checked)"
fi
exit "$rc"
