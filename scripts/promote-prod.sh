#!/usr/bin/env bash
# Pin every prod values file to a release commit BY DIGEST, and label the images
# with the CalVer (ADR-0103, ADR-0201). A digest cannot be re-pushed, so a pod
# cannot silently run different code under the same reference; the CalVer is a
# human-facing label on the image and never what a values file references.
#
# REPO is the registry namespace, SHA the release commit, VER the CalVer tag.
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/log.sh"
# shellcheck source=lib/yaml.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/yaml.sh"

sha="${SHA:-}"
ver="${VER:-}"
[[ -n "$sha" ]] || fail "SHA is unset"
[[ -n "$ver" ]] || fail "VER is unset"

# image.repository is read from each values file, so this covers both the
# -server/-worker services and the apps (the frontend has no suffix).
pin() { # <values-file> <yaml-path-to-image-block>
  local path="$1" block="$2" repo digest
  [[ -f "$path" ]] || return 0
  repo="$(yq "${block}.repository // \"\"" "$path")"
  [[ -n "$repo" ]] || return 0
  digest="$(docker buildx imagetools inspect "${repo}:${sha}" --format '{{.Manifest.Digest}}')"
  yaml_set_scalar "$path" "${block}.digest" "$digest"
  yaml_set_scalar "$path" "${block}.tag" "$sha"
  docker buildx imagetools create -t "${repo}:${ver}" "${repo}:${sha}"
  detail "${repo} → ${digest} (labelled ${ver})"
}

shopt -s nullglob
step "pinning prod to ${sha}"
for path in infra/gitops/services/prod/values/*.yaml; do
  pin "$path" ".image"
  pin "$path" ".worker.image"
done
# The admin console is the one repo-built platform image (lowdefy chart, ADR-0401).
pin "infra/gitops/platform/prod/values.yaml" ".lowdefy.image"

ok "prod pinned to ${sha}, images labelled ${ver}"
