#!/usr/bin/env bash
# Give this project its own local-tier age key (ADR-0202, ADR-0205).
#
#   mise run secrets:age:local          # rotate only if the key is still the template's
#   mise run secrets:age:local -- --force
#
# The local tier's private key is COMMITTED, which ADR-0202 permits because it
# decrypts throwaway values in a cluster holding no real data. What the exemption
# does not permit is every project generated from this template sharing one key —
# that is a default credential, and it stays harmless only while each project keeps
# remembering the key is throwaway. The first person to encrypt something real to it
# has no way to know that reasoning ever existed.
#
# So the key is rotated on first use. `cluster:up` calls this before it decrypts
# anything, and a project that has already rotated is left alone.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

KEY_FILE="infra/gitops/platform/local/age.key"
SOPS_FILE=".sops.yaml"
SECRETS_GLOB="infra/gitops/platform/local/secrets"

# The recipient this template ships. A key matching it has not been rotated, which
# is the whole condition this script exists to detect. Recorded here rather than in
# a separate file so there is one place to update if the shipped key ever changes.
TEMPLATE_RECIPIENT="age1aapmzzzcs8qm2n9exkel8vtknqsmy9t3k9g99wpypfez8av4uaps0f9gpp"

FORCE=false
[ "${1:-}" != "--force" ] || FORCE=true

[ -f "$KEY_FILE" ] || fail "$KEY_FILE does not exist"

current="$(age-keygen -y "$KEY_FILE")"

if [ "$FORCE" = false ] && [ "$current" != "$TEMPLATE_RECIPIENT" ]; then
  ok "the local age key is this project's own (${current:0:20}…)"
  exit 0
fi

step "minting a local age key for this project"

# The header explaining WHY a private key is committed has to survive the rotation.
# `age-keygen -o` writes its own file, so the header is re-attached rather than
# preserved.
header="$(grep '^#' "$KEY_FILE" || true)"
tmp="$(mktemp)"
old="$(mktemp)"
both="$(mktemp)"
trap 'rm -f "$tmp" "$old" "$both"' EXIT

# The OLD key has to outlive the overwrite below. `sops updatekeys` re-encrypts to
# the new recipient set, but it must DECRYPT the file first, and only the old key
# can do that — rotating the file on disk before re-encrypting would strand every
# value in it.
cp "$KEY_FILE" "$old"

# `age-keygen -o` REFUSES to overwrite an existing file, and mktemp has already
# created one — so the name is reserved and then cleared before the key is written.
rm -f "$tmp"
age-keygen -o "$tmp" 2>/dev/null
chmod 600 "$tmp"
new_recipient="$(age-keygen -y "$tmp")"

{
  printf '%s\n' "$header"
  grep -v '^#' "$tmp"
} >"$KEY_FILE"
chmod 600 "$KEY_FILE"

step "pointing .sops.yaml's cluster_local recipient at it"
# Anchored to the anchor name, so this cannot rewrite one of the other four
# recipients if the file is reordered.
sed -i -E "s|(&cluster_local[[:space:]]+)age1[a-z0-9]+|\1${new_recipient}|" "$SOPS_FILE"

grep -q "$new_recipient" "$SOPS_FILE" ||
  fail ".sops.yaml still does not name the new recipient — the cluster_local anchor was not found"

# Re-encrypt with the OLD key still available to read the existing values, which is
# what `sops updatekeys` needs: it decrypts with a current recipient and re-encrypts
# to the new recipient set.
step "re-encrypting the local secrets to the new recipient"
shopt -s nullglob
files=("$SECRETS_GLOB"/*.enc.yaml)
shopt -u nullglob

if [ ${#files[@]} -eq 0 ]; then
  # Not a silent pass. The template ships an encrypted file here, so finding none
  # means either the path moved or a project deleted it — and in the second case the
  # rotation above just orphaned whatever recipients that file had.
  fail "no encrypted files under ${SECRETS_GLOB}/ — expected at least one"
fi

# Both keys in one file: sops reads every identity it is given, so the old one
# opens the file and the new one is what .sops.yaml now says to close it with.
cat "$old" "$tmp" >"$both"

for f in "${files[@]}"; do
  SOPS_AGE_KEY_FILE="$both" sops updatekeys --yes "$f" >/dev/null
  printf '  %s\n' "$f"
done

ok "local age key rotated to ${new_recipient:0:20}…"
printf '  The shared template key no longer opens this project.\n'
