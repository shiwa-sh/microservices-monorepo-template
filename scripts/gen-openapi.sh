#!/usr/bin/env bash
# Regenerate Go servers/clients and TS clients from every service's OpenAPI spec.
# Outputs land in libs/{go,ts}/sdks/<service>/ (ADR-0008).
#   - Go:  ogen emits one type-safe package per spec (server + client + types,
#          with built-in OpenTelemetry instrumentation).
#   - TS:  openapi-typescript emits the types consumed via openapi-fetch.
set -euo pipefail

shopt -s nullglob

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

for spec in services/*/openapi.yaml; do
  service=$(basename "$(dirname "$spec")")
  go_out="libs/go/sdks/${service}"
  ts_out="libs/ts/sdks/${service}"
  # ogen package name must be a valid Go identifier (e.g. "_template" -> "template").
  pkg=$(printf '%s' "$service" | tr -cd '[:alnum:]')

  echo "→ $service: Go SDK (ogen)"
  rm -rf "$go_out"
  mkdir -p "$go_out"
  ogen --target "$go_out" --package "$pkg" --clean "$spec"

  echo "→ $service: TS client"
  mkdir -p "$ts_out"

  bun x openapi-typescript@7.13.0 "$spec" --output "$ts_out/index.d.ts"
done
