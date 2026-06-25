#!/usr/bin/env bash
# SpiceDB schema + assertion validation (ADR-0010). Assembles the canonical
# split sources — schema.zed (the one schema source), validations.yaml (sample
# relationships) and assertions.yaml (assertTrue/assertFalse) — into the single
# validation document `zed validate` expects, then runs it. Keeping the schema in
# schema.zed avoids duplicating it in a combined playground file.
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd)"

DIR="infra/auth/spicedb"
tmp="$(mktemp --suffix=.yaml)"
trap 'rm -f "$tmp"' EXIT

{
  echo "schema: |"
  sed 's/^/  /' "$DIR/schema.zed"
  cat "$DIR/validations.yaml"
  # zed expects assertTrue/assertFalse nested under `assertions:`; the canonical
  # assertions.yaml keeps them top-level, so indent the whole file one level.
  echo "assertions:"
  sed 's/^/  /' "$DIR/assertions.yaml"
} > "$tmp"

zed validate --type yaml "$tmp"
