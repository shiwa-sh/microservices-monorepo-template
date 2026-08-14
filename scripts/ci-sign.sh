#!/usr/bin/env bash
# Sign a published image and attach its SBOM and provenance (ADR-0104).
#
#   mise run ci:sign -- ghcr.io/org/repo/catalog-server@sha256:…
#
# Three artefacts, one command, because they are one decision: an image is
# admissible when it carries a signature this platform made, a bill of materials
# saying what is in it, and provenance saying where it came from. Splitting them
# across three workflow steps is how one of them silently stops running.
#
# THE REFERENCE MUST BE A DIGEST. cosign resolves a tag to a digest and signs that,
# so signing `:v1.2.3` signs whatever the tag pointed at when cosign looked — which
# is the race the digest-pinning rule exists to remove. Passing a tag here is
# refused rather than resolved, because resolving it would reintroduce the gap in
# the one place nobody would look for it.
#
# THE KEY IS DECRYPTED HERE AND NOWHERE ELSE. CI holds an age key as a forge secret
# (SOPS_AGE_KEY), which decrypts exactly one file: infra/auth/cosign. The cosign
# private key exists as a file inside a directory this script owns for the length of
# the signing run, removed on the way out including on failure. It is never written
# to the workspace, never passed as an argument, and never printed.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

IMAGE="${1:-}"
[ -n "$IMAGE" ] || fail "usage: mise run ci:sign -- <image>@sha256:<digest>"
case "$IMAGE" in
*@sha256:*) ;;
*) fail "refusing to sign a tag: pass an image by digest (ADR-0104). Got: ${IMAGE}" ;;
esac

PRIVATE="infra/auth/cosign/signing-key.enc.yaml"
[ -f "$PRIVATE" ] || fail "no signing key at ${PRIVATE} — run 'mise run secrets:cosign' once, at bootstrap"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
chmod 700 "$work"

step "decrypting the signing key"
sops -d "$PRIVATE" >"$work/secret.yaml"
yq -r '.stringData["cosign.key"]' "$work/secret.yaml" >"$work/cosign.key"
COSIGN_PASSWORD="$(yq -r '.stringData.COSIGN_PASSWORD' "$work/secret.yaml")"
export COSIGN_PASSWORD
rm -f "$work/secret.yaml"

step "signing ${IMAGE}"
cosign sign --yes --key "$work/cosign.key" "$IMAGE"

# A file, not a pipe: syft writes progress to stderr and the document to stdout, and
# a pipeline that swallows one loses the other's failure with it.
step "generating the SPDX SBOM"
syft "$IMAGE" -o "spdx-json=$work/sbom.spdx.json"

step "attaching the SBOM and the provenance"
cosign attest --yes --key "$work/cosign.key" \
  --type spdxjson --predicate "$work/sbom.spdx.json" "$IMAGE"

# SLSA provenance: what source, what builder. The predicate is assembled here from
# the forge's own environment rather than taken from a builder attestation, because
# the builder is `docker/build-push-action` and the claim being recorded is this
# pipeline's, not BuildKit's.
#
# `startedOn` defaults to now rather than to the empty string: the predicate is
# parsed as a protobuf Timestamp, and "" is not one — the attestation is rejected
# with a message about google.protobuf.Timestamp, four steps from the cause.
jq -n \
  --arg repo "${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-unknown}" \
  --arg sha "${GITHUB_SHA:-unknown}" \
  --arg ref "${GITHUB_REF:-unknown}" \
  --arg run "${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-unknown}/actions/runs/${GITHUB_RUN_ID:-0}" \
  --arg started "${BUILD_STARTED_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
  '{
     buildDefinition: {
       buildType: "https://github.com/actions/runner",
       externalParameters: { repository: $repo, ref: $ref },
       resolvedDependencies: [ { uri: $repo, digest: { gitCommit: $sha } } ]
     },
     runDetails: {
       builder: { id: $run },
       metadata: { invocationId: $run, startedOn: $started }
     }
   }' >"$work/provenance.json"

cosign attest --yes --key "$work/cosign.key" \
  --type slsaprovenance1 --predicate "$work/provenance.json" "$IMAGE"

ok "signed, SBOM and provenance attached: ${IMAGE}"
