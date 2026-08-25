#!/usr/bin/env bash
# Mirror the first-party images into the FULL tier's in-cluster zot (ADR-0105).
#
#   mise run cluster:populate-zot
#
# The nodes pull from the host container `registry.localhost` because a
# `*.localtest.me` name does not resolve inside a node, so the in-cluster zot —
# the registry a deployed environment runs — never sees an image and its
# `zot.ops` console reads empty. This copies the repo's own images (everything
# tagged `:local`, which is exactly what `stage_images` built) from the host
# container into the in-cluster zot, so its push, object-store, and console path is
# exercised locally rather than only in a deployed environment. The upstream mirror
# images are left where they are — the nodes' pull path is unchanged.
#
# Called as the `populatezot` stage of `cluster:up full` after the platform is
# healthy, and runnable on its own. Best-effort: a copy failure is a warning, never
# a failed bring-up, because the console is a convenience and the node pull path
# does not depend on it.
set -euo pipefail

source "$(dirname "$0")/lib/cluster.sh"

# The full tier is the only one running an in-cluster zot; on base it is a no-op.
[ "$TIER" = full ] || {
  detail "populate-zot: no in-cluster zot on the ${TIER} tier — skipping"
  exit 0
}

# The local push identity. Its plaintext is not in the Secret (only the bcrypt
# htpasswd is), so it is named here — a committed throwaway, the same category as
# every other local credential (ADR-0205). Override for a non-default local secret.
PUSH_USER="${ZOT_LOCAL_PUSH_USER:-ci}"
PUSH_PASSWORD="${ZOT_LOCAL_PUSH_PASSWORD:-ci-local-push}"

SRC="127.0.0.1:5000" # the host container `registry.localhost`, insecure on 127.0.0.1
FWD_PORT="${ZOT_FWD_PORT:-5100}"
DST="127.0.0.1:${FWD_PORT}"

command -v oras >/dev/null || fail "oras is not on PATH — run through mise so the pinned tool resolves"

k -n "$NS" rollout status deploy/zot --timeout=120s >/dev/null 2>&1 ||
  {
    warn "populate-zot: zot is not ready — skipping"
    exit 0
  }

step "mirroring first-party images into the in-cluster zot"

# Port-forward the registry API (5000), not the console proxy (5001): a push
# authenticates against zot's own htpasswd directly, the way CI does.
k -n "$NS" port-forward svc/zot "${FWD_PORT}:5000" >/dev/null 2>&1 &
fwd_pid=$!
trap 'kill "$fwd_pid" 2>/dev/null || true' EXIT
for _ in $(seq 1 30); do
  if curl -sf -o /dev/null "http://${DST}/v2/"; then break; fi
  sleep 0.5
done

o_src=(--plain-http)
o_dst=(--to-plain-http --to-username "$PUSH_USER" --to-password "$PUSH_PASSWORD")

copied=0 skipped=0
# `:local` is the tag `stage_images` gives every repo image it builds, and nothing
# else carries it — so this selects exactly the first-party set without a hand list.
for repo in $(oras repo ls "${o_src[@]}" "$SRC" 2>/dev/null); do
  oras repo tags "${o_src[@]}" "${SRC}/${repo}" 2>/dev/null | grep -qx local || {
    skipped=$((skipped + 1))
    continue
  }
  # Two attempts: zot answers a blob PUT with a 500 when its object-store write
  # blips under a concurrent copy, and the retry rides over it — oras skips the
  # layers already there, so a retry copies only what the first pass missed.
  if oras cp --from-plain-http "${o_dst[@]}" "${SRC}/${repo}:local" "${DST}/${repo}:local" >/dev/null 2>&1 ||
    oras cp --from-plain-http "${o_dst[@]}" "${SRC}/${repo}:local" "${DST}/${repo}:local" >/dev/null 2>&1; then
    detail "mirrored ${repo}:local"
    copied=$((copied + 1))
  else
    warn "could not mirror ${repo}:local — the console will be missing it"
  fi
done

ok "in-cluster zot populated: ${copied} first-party image(s) mirrored, ${skipped} upstream repo(s) left in place"
