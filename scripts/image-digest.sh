#!/usr/bin/env bash
# Resolve a tag to the immutable index digest a values file or Dockerfile pins to
# (ADR-0101, ADR-0104). Reads the registry's own `Docker-Content-Digest` for the
# tag rather than hashing a manifest body, because a multi-arch tag's body is the
# index and hashing one platform's manifest pins one architecture by accident.
# Progress goes to stderr so stdout is the digest alone and the call composes.
#   mise run image:digest -- gcr.io/distroless/static-debian13:nonroot
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/log.sh"

ref="${1:-}"
[[ -n "$ref" ]] || fail "usage: mise run image:digest -- <registry>/<repo>:<tag>"

case "$ref" in
*@sha256:*) fail "already digest-pinned: $ref" ;;
*:*)
  tag="${ref##*:}"
  name="${ref%:*}"
  ;;
*) fail "no tag in reference: $ref — a floating reference has nothing to pin" ;;
esac

case "$name" in
*/*/*)
  registry="${name%%/*}"
  repo="${name#*/}"
  ;;
*)
  registry="registry-1.docker.io"
  repo="library/${name}"
  ;;
esac

accept='application/vnd.oci.image.index.v1+json'
accept+=',application/vnd.docker.distribution.manifest.list.v2+json'
accept+=',application/vnd.oci.image.manifest.v1+json'
accept+=',application/vnd.docker.distribution.manifest.v2+json'

step "resolving ${ref}" >&2
digest="$(curl -fsSL -o /dev/null -D - -H "Accept: ${accept}" \
  "https://${registry}/v2/${repo}/manifests/${tag}" |
  tr -d '\r' | awk 'tolower($1) == "docker-content-digest:" { print $2 }')"

[[ -n "$digest" ]] || fail "registry returned no digest for ${ref}"
ok "${name}:${tag}@${digest}" >&2
printf '%s\n' "$digest"
