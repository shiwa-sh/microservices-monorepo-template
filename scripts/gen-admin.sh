#!/usr/bin/env bash
# Regenerate the Lowdefy admin pages from the service OpenAPI specs (ADR-0401).
# Output lands in apps/admin/_generated/ (one page per resource/action + a
# pages.yaml manifest). REST-connector pages only — the write-path invariant.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

echo "→ admin pages (Lowdefy) from services/*/openapi.yaml"
go run ./tools/admin-gen
