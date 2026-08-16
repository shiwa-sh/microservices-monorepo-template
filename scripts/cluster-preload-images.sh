#!/usr/bin/env bash
# Preload every platform image into the node BEFORE the imperative bring-up needs
# them (OPT-IN, proxied networks only).
#
# The sibling cluster:unwedge is REACTIVE — it rescues Argo-synced pods stuck in
# ImagePullBackOff. But cluster:full's imperative head (cilium-install, then the
# ArgoCD install) BLOCKS on `helm --wait` rollouts BEFORE Argo ever runs, so if
# the node's containerd wedges pulling Cilium or ArgoCD through a slow proxy there
# is no pod loop for unwedge to fix — the whole bring-up just hangs. This warms
# every platform image ahead of time: host `docker pull` (the host proxy handles
# large TLS blobs fine) → `kind load docker-image` into the node's containerd.
#
#   mise run cluster:preload          # every platform chart image
#
# Nothing is hardcoded: the image list is derived from the charts via `helm
# template`, so it tracks chart image bumps. The clean-network path never calls
# this — cluster:full only runs it when CLUSTER_PRELOAD=1.
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# cilium + argocd are the two charts cluster:full installs imperatively (helm
# --wait) before Argo, so a wedged pull there stalls the whole bring-up; the rest
# Argo pulls later (cluster:unwedge can also rescue those reactively). We preload
# every platform chart so nothing has to wedge first — cilium/argocd lead so the
# bootstrap-critical images land even if a later chart's pull is slow.
charts=(cilium argocd)
for d in infra/helm/platform/*/; do
  name="$(basename "$d")"
  case "$name" in cilium | argocd) continue ;; esac
  charts+=("$name")
done

# Best-effort per-chart render → image refs. cilium needs a couple of required
# values to template; the rest render with their own defaults. A chart that will
# not template contributes nothing (its images fall to cluster:unwedge later) and
# is reported, never silently dropped.
render() { # <chart>
  local c="$1"
  case "$c" in
  cilium) helm template cilium infra/helm/platform/cilium \
    --set cilium.k8sServiceHost=127.0.0.1 2>/dev/null ;;
  *) helm template "$c" "infra/helm/platform/$c" 2>/dev/null ;;
  esac
}

echo "→ deriving image list from charts: ${charts[*]}"
declare -a want=()
for c in "${charts[@]}"; do
  [ -d "infra/helm/platform/$c" ] || continue
  mapfile -t imgs < <(render "$c" | grep -oE 'image:[[:space:]]*"?[^"]+' | sed -E 's/image:[[:space:]]*"?//' | grep -E '/' | sort -u)
  if [ "${#imgs[@]}" -eq 0 ]; then
    echo "  ! $c: no images rendered (skipped — cluster:unwedge covers it if it wedges)"
    continue
  fi
  want+=("${imgs[@]}")
done
mapfile -t want < <(printf '%s\n' "${want[@]}" | sort -u)

if [ "${#want[@]}" -eq 0 ]; then
  echo "✗ derived no images — is helm on PATH?" >&2
  exit 1
fi

# Host-pull with two proxy-era hazards handled: (1) the slow proxy trips docker's
# TLS-handshake timeout, so retry; (2) a stale ~/.docker/config.json entry for a
# PUBLIC registry makes an anonymous pull fail with "denied" — one `docker logout`
# of that host clears it (we only ever pull public images here).
docker_pull() { # <ref>
  local ref="$1" host="${1%%/*}" out
  case "$host" in *.* | *:*) ;; *) host="docker.io" ;; esac # bare name → Docker Hub
  for attempt in 1 2 3; do
    if out=$(docker pull "$ref" 2>&1); then return 0; fi
    printf '%s\n' "$out" | tail -1
    if printf '%s' "$out" | grep -qiE 'denied|unauthorized' && [ "$attempt" = 1 ]; then
      echo "    (auth denied for a public image — docker logout $host and retry)"
      docker logout "$host" >/dev/null 2>&1 || true
    fi
    echo "    (pull attempt $attempt failed — retrying)"
  done
  return 1
}

echo "→ preloading ${#want[@]} image(s) into node '$CLUSTER'"
failed=()
for img in "${want[@]}"; do
  echo "  · $img"
  if ! docker_pull "$img"; then
    echo "    ✗ could not pull $img — skipping (cluster:unwedge can retry later)"
    failed+=("$img")
    continue
  fi
  # kind load docker-image can't resolve a `repo:tag@sha256:…` ref; a digest-only
  # pull leaves no `repo:tag` local tag either. Tag it back so the load (and the
  # digest-pinned pod) resolve the same bytes locally. Same dance as cluster:unwedge.
  import_ref="$img"
  if [[ "$img" == *"@sha256:"* ]]; then
    no_digest="${img%@sha256:*}"
    if [[ "${no_digest##*/}" == *:* ]]; then
      docker tag "$img" "$no_digest" && import_ref="$no_digest"
    fi
  fi
  kind load docker-image "$import_ref" --name "$CLUSTER" >/dev/null || {
    echo "    ✗ import failed: $import_ref"
    failed+=("$img")
  }
done

ok=$((${#want[@]} - ${#failed[@]}))
echo "✓ preloaded ${ok}/${#want[@]} image(s) into the node"
if [ "${#failed[@]}" -gt 0 ]; then
  echo "✗ ${#failed[@]} image(s) not preloaded (proxy still choking) — re-run to retry:" >&2
  printf '    · %s\n' "${failed[@]}" >&2
  exit 1
fi
