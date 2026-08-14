#!/usr/bin/env bash
# Generate the platform's image-signing key pair (ADR-0104, ADR-0202).
#
#   mise run secrets:cosign
#
# ONE pair for the platform, not one per environment. An image is built once and
# the same digest flows dev → staging → prod (ADR-0103), so a per-environment key
# would mean an image that verifies in dev and is rejected in prod — the promotion
# model and per-environment signing keys cannot both be true.
#
# A task rather than a documented sequence of commands, and that is the decision
# ADR-0104 left open. The sequence is four steps that must all happen — generate,
# encrypt the private half, commit the public half where the policy reads it, and
# never let the private half touch the disk in the clear — and a documented sequence
# is one where the fourth step is the one someone skips. Here the private key exists
# as a file for the length of one `sops --encrypt`, in a directory this script owns
# and removes on the way out, including on failure.
#
# WHO HOLDS WHAT. The private half is decrypted by CI, which signs, and by nobody
# else: no cluster ever needs it, because Kyverno verifies with the public half.
# That is why it lands beside the other canonical auth material in infra/auth/
# rather than in an environment's SopsSecret bundle — a secret delivered to a
# cluster is a secret that cluster can be made to read.
#
# WHAT THIS DOES NOT DO is rotate. Rotation has an ordering constraint — the policy
# must trust both keys through the window, or the cluster stops being able to
# restart its own running images — and it is six steps in
# docs/guide/secrets-runbook.md.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

KEY_DIR="infra/auth/cosign"
PUB_KEY="${KEY_DIR}/cosign.pub"
PRIVATE="${KEY_DIR}/signing-key.enc.yaml"

if [ -f "$PUB_KEY" ] || [ -f "$PRIVATE" ]; then
  fail "${KEY_DIR} already holds a key pair. Generating a second one is a ROTATION — follow docs/guide/secrets-runbook.md, which carries both public keys through the window."
fi

# The recipients come from .sops.yaml. Refusing early with the real reason beats
# failing inside sops with "malformed recipient": the template ships placeholder
# age keys, so this is the first thing a new project hits.
if grep -q 'age1example' .sops.yaml; then
  fail ".sops.yaml still carries placeholder recipients. Add your real engineer and CI age keys first (mise run secrets:age, then .sops.yaml) — a signing key encrypted to a placeholder is a signing key nobody can decrypt."
fi

mkdir -p "$KEY_DIR"

# cosign writes cosign.key/cosign.pub into the working directory and takes the
# passphrase from COSIGN_PASSWORD. The passphrase is generated here too: it is
# encrypted beside the key it protects, so a memorable one buys nothing and a
# guessable one costs everything.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
password="$(head -c 32 /dev/urandom | base64 | tr -d '\n=' | head -c 32)"

step "generating the platform image-signing key pair"
(cd "$work" && COSIGN_PASSWORD="$password" cosign generate-key-pair >/dev/null)

cp "$work/cosign.pub" "$PUB_KEY"
ok "public key → ${PUB_KEY} (committed in the clear; every environment's Kyverno policy names it)"

step "encrypting the private half into ${PRIVATE}"
COSIGN_KEY="$(cat "$work/cosign.key")" COSIGN_PASSWORD="$password" \
  yq -n '{
    "apiVersion": "v1",
    "kind": "Secret",
    "metadata": { "name": "cosign-signing-key" },
    "stringData": {
      "cosign.key": strenv(COSIGN_KEY),
      "COSIGN_PASSWORD": strenv(COSIGN_PASSWORD)
    }
  }' >"$work/plain.yaml"
# --filename-override so sops picks the creation rule for the DESTINATION path: it
# resolves rules against the input file's name, and the input is a temporary file
# outside the repo — which matches no rule and fails with "no matching creation
# rules found".
sops --encrypt --filename-override "$PRIVATE" "$work/plain.yaml" >"$PRIVATE"

ok "private key + passphrase → ${PRIVATE}"
echo
echo "Next:"
echo "  1. Commit both halves. The public one is plaintext by design."
echo "  2. Kyverno's ClusterPolicy names ${PUB_KEY} as its attestor (infra/helm/platform/kyverno)."
echo "  3. Give CI an age recipient in .sops.yaml and its private half as a forge"
echo "     secret, so ci:sign can decrypt this file. Nothing else ever decrypts it."
