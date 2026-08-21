#!/usr/bin/env bash
# Preload every platform image into the node BEFORE the imperative bring-up needs
# them (OPT-IN, proxied networks only).
#
# Works on both tiers, by different means: `kind load` into the inner loop's node,
# and a push to the local registry for the full tier, which has no load path. See
# import_image below.
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

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
source "$(dirname "$0")/lib/cluster-ctx.sh"

CLUSTER="$(cluster_name)"

# How an image reaches the node differs by tier, and on the full tier there is no
# load path at all — a Talos node holds nothing the cluster did not PULL.
#
# So the full tier's preload is a push: the image is fetched on the host, where the
# proxy is configured and the pull is serial, then pushed to the local registry.
# The nodes reach it because infra/talos/local/patch.yaml mirrors quay.io, ghcr.io,
# registry.k8s.io and docker.io at that registry, with upstream fallback left on.
#
# The repository path is preserved across the push — `quay.io/argoproj/argocd`
# becomes `registry.localhost:5000/argoproj/argocd` — because a containerd mirror
# requests the SAME path from the mirror endpoint. Rewriting the path would produce
# a registry full of images no node ever asks for.
LOCAL_REGISTRY="127.0.0.1:5000"

import_image() { # <ref>
  local ref="$1"
  if [ "$(cluster_tier)" != "full" ]; then
    kind load docker-image "$ref" --name "$CLUSTER" >/dev/null
    return
  fi
  # The repository path the NODE asks the mirror for: the ref minus its registry
  # host. Three shapes, and conflating them silently wastes the push:
  #
  #   redis:8              → library/redis:8     (Docker Hub's implicit namespace)
  #   axllent/mailpit:v1   → axllent/mailpit:v1  (already namespaced — NOT library/)
  #   ghcr.io/org/app:v1   → org/app:v1          (explicit host, stripped)
  #
  # `library/` applies only to a SINGLE-segment name. Adding it to `axllent/mailpit`
  # produces `library/axllent/mailpit`, which no node ever requests — the pull falls
  # back upstream and the preload bought nothing. Measured: mailpit and seaweedfs
  # both landed under the wrong path.
  local path
  if [[ "$ref" != */* ]]; then
    path="library/${ref}"
  elif [[ "${ref%%/*}" == *.* || "${ref%%/*}" == *:* ]]; then
    path="${ref#*/}"
  else
    path="$ref"
  fi
  docker tag "$ref" "${LOCAL_REGISTRY}/${path}"
  docker push -q "${LOCAL_REGISTRY}/${path}" >/dev/null
}

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

# THE NODE'S OWN IMAGES come first, and they are a different class from the charts
# below: the kubelet, the API server, etcd and pause are pulled by Talos during
# BOOTSTRAP, before Kubernetes exists at all. A chart image that cannot be pulled
# leaves a pod in ImagePullBackOff, which is visible and recoverable; one of these
# leaves `etcd` and `kubelet` in `Preparing`, the API server never answers, and
# every tool reports a timeout against a cluster that never came up.
#
# `talosctl image default` is the list for the pinned Talos version, so this tracks
# a version bump the same way the chart list tracks an image bump — by asking
# rather than by being told.
declare -a node_images=()
if [ "$(cluster_tier)" = "full" ]; then
  mapfile -t node_images < <(talosctl image default 2>/dev/null || true)
  if [ "${#node_images[@]}" -gt 0 ]; then
    echo "→ warming the node's own bootstrap images (${#node_images[@]})"
    for ref in "${node_images[@]}"; do
      [ -n "$ref" ] || continue
      docker pull -q "$ref" >/dev/null 2>&1 || {
        echo "  ! $ref: pull failed"
        continue
      }
      import_image "$ref"
    done
  fi
fi

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
  import_image "$import_ref" || {
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
