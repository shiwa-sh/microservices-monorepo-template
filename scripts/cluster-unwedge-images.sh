#!/usr/bin/env bash
# Unwedge image pulls stalled by an HTTP proxy (OPT-IN, proxied networks only).
#
# The normal bring-up is proxy-free: on a clean network every image pulls fine and
# you never need this. Behind a corporate proxy, though, the kind node's containerd
# pulls public-registry images (cilium, argocd, openfga/openfga, …) THROUGH privoxy,
# which times out on large TLS blobs and can wedge the pull indefinitely — pods sit
# in ImagePullBackOff / ErrImagePull. See docs/dev-loop.md ("HTTP proxies").
#
# This does the documented manual recovery, automatically and only for what is
# actually stuck: host `docker pull` (the host proxy handles blobs fine) → then
# `kind load docker-image` into the node's containerd → then delete the stuck pod
# so it retries against the now-local image. Nothing is hardcoded, so it tracks
# chart image bumps and covers whatever the proxy happened to choke on this run.
#
#   mise run cluster:unwedge          # one pass
#   watch -n15 mise run cluster:unwedge   # or loop while a fresh cluster:full converges
#
# The repo's default path never calls this — it exists solely so a proxied machine
# has a working, explicit escape hatch without polluting the clean-network setup.
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
source "$(dirname "$0")/lib/cluster-ctx.sh"
k() { kubectl --context "$(cluster_ctx)" "$@"; }

# Every image referenced by a container that is currently failing to pull, across
# all namespaces — init and regular containers alike. `waiting.reason` is the
# authoritative signal (ImagePullBackOff/ErrImagePull); dedupe since one image can
# wedge many pods.
readarray -t stuck < <(
  k get pods -A -o json |
    jq -r '
        .items[].status
        | (.initContainerStatuses // []) + (.containerStatuses // [])
        | .[]
        | select(.state.waiting.reason // "" | test("ImagePull|ErrImagePull"))
        | .image
      ' |
    sort -u
)

if [ "${#stuck[@]}" -eq 0 ]; then
  echo "✓ no image pulls are stuck — nothing to unwedge"
  exit 0
fi

echo "→ ${#stuck[@]} image(s) stuck behind the proxy; pulling on the host + importing:"
# The proxy is slow, so a single host pull can trip docker's TLS-handshake timeout
# even when the image is perfectly reachable — retry a few times before giving up.
docker_pull() {
  local ref="$1" host="${1%%/*}" out
  case "$host" in *.* | *:*) ;; *) host="docker.io" ;; esac # bare name → Docker Hub
  for attempt in 1 2 3; do
    if out=$(docker pull "$ref" 2>&1); then return 0; fi
    printf '%s\n' "$out" | tail -1
    # "manifest unknown"/"not found" isn't a proxy stall — the image simply was
    # never built+pushed to the registry (e.g. a new service/worker missing from
    # cluster-full.sh's build loop). Retrying against the proxy can't conjure it,
    # so bail immediately with a distinct code so the caller can say so plainly.
    if printf '%s' "$out" | grep -qiE 'manifest unknown|manifest .* not found|not found: manifest'; then
      return 2
    fi
    # A stale ~/.docker/config.json entry for a PUBLIC registry fails an anonymous
    # pull with "denied"; one logout of that host clears it (we only pull public
    # images here).
    if printf '%s' "$out" | grep -qiE 'denied|unauthorized' && [ "$attempt" = 1 ]; then
      echo "    (auth denied for a public image — docker logout $host and retry)"
      docker logout "$host" >/dev/null 2>&1 || true
    fi
    echo "    (pull attempt $attempt failed — retrying)"
  done
  return 1
}

# One unreachable image must not abort the batch: import everything that does pull,
# collect the ones that don't, and fail loudly at the end so nothing is swallowed.
# `missing` is kept separate from `failed`: a not-found image is a build/config gap
# (the wrong diagnosis for this tool), not a proxy stall — re-running won't help.
failed=()
missing=()
for img in "${stuck[@]}"; do
  echo "  · $img"
  # Pull by the full, digest-pinned ref so we fetch exactly the bytes the pod wants.
  docker_pull "$img"
  rc=$?
  if [ "$rc" = 2 ]; then
    echo "    ✗ $img not found in registry — never built/pushed (not a proxy issue)"
    missing+=("$img")
    continue
  elif [ "$rc" != 0 ]; then
    echo "    ✗ could not pull $img — skipping"
    failed+=("$img")
    continue
  fi
  # kind load docker-image can't resolve a combined `repo:tag@sha256:…` ref (its
  # runtime lookup only matches plain tags), so load by the plain tag. But
  # `docker pull` of a digest-pinned ref stores the image under `repo@sha256:…`
  # only — it does NOT create the `repo:tag` tag — so we must tag it ourselves
  # first, else the load fails. The content digest is unchanged, so the
  # digest-pinned pod still resolves it locally.
  import_ref="$img"
  if [[ "$img" == *"@sha256:"* ]]; then
    no_digest="${img%@sha256:*}"
    if [[ "${no_digest##*/}" == *:* ]]; then
      docker tag "$img" "$no_digest"
      import_ref="$no_digest"
    fi
  fi
  if ! kind load docker-image "$import_ref" --name "$CLUSTER"; then
    echo "    ✗ could not import $import_ref — skipping"
    failed+=("$img")
  fi
done

# Nudge the stuck pods to retry now (delete Pending/waiting pods; their owners —
# Jobs, Deployments, DaemonSets — recreate them, which re-pulls and finds the image
# local). Only pods with a still-waiting pull container are touched.
echo "→ restarting pods that were waiting on those images"
k get pods -A -o json |
  jq -r '
      .items[]
      | select(
          [ (.status.initContainerStatuses // []) + (.status.containerStatuses // [])
            | .[] | .state.waiting.reason // "" ]
          | any(test("ImagePull|ErrImagePull"))
        )
      | "\(.metadata.namespace) \(.metadata.name)"
    ' |
  while read -r ns pod; do
    echo "  · $ns/$pod"
    k -n "$ns" delete pod "$pod" --wait=false
  done

imported=$((${#stuck[@]} - ${#failed[@]} - ${#missing[@]}))
echo "✓ imported ${imported}/${#stuck[@]} image(s); pods will re-pull locally. Re-run if more wedge."
if [ "${#missing[@]}" -gt 0 ]; then
  echo "✗ ${#missing[@]} image(s) not found in the registry — never built/pushed."
  echo "  This is NOT a proxy stall; re-running won't help. Build the image (usually a"
  echo "  service/worker missing from cluster-full.sh's build loop) and push it:"
  printf '    · %s\n' "${missing[@]}"
fi
if [ "${#failed[@]}" -gt 0 ]; then
  echo "✗ ${#failed[@]} image(s) could not be pulled (proxy still choking) — re-run to retry:"
  printf '    · %s\n' "${failed[@]}"
fi
[ "$((${#missing[@]} + ${#failed[@]}))" -gt 0 ] && exit 1
exit 0
