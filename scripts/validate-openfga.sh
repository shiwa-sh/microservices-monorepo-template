#!/usr/bin/env bash
# OpenFGA model + assertion validation (ADR-0304). model.fga is the one source of
# truth; model.json is a generated artifact the seed Job posts to the API. This
# runs the store tests (fga.yaml) against the model AND asserts model.json is in
# sync with model.fga, so a hand-edited DSL can never drift from the deployed JSON.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

DIR="infra/auth/openfga"

step "validating OpenFGA model + assertions with fga model test"
fga model test --tests "$DIR/fga.yaml"

step "checking $DIR/model.json is in sync with model.fga"
fresh="$(mktemp --suffix=.json)"
trap 'rm -f "$fresh"' EXIT
fga model transform --file "$DIR/model.fga" --output-format json | jq -S . >"$fresh"
if ! diff -u "$DIR/model.json" "$fresh"; then
  fail "model.json is stale — run 'mise run gen:authz-model' and commit the result"
fi
ok "OpenFGA model valid and model.json in sync"
