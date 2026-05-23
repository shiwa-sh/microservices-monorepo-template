#!/usr/bin/env bash
# Regenerate Go servers/clients and TS clients from every service's OpenAPI spec.
# Outputs land in libs/sdks/{go,ts}/<service>/ (ADR-0008).
set -euo pipefail

shopt -s nullglob

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

for spec in services/*/openapi.yaml; do
  service=$(basename "$(dirname "$spec")")
  go_out="libs/sdks/go/${service}"
  ts_out="libs/sdks/ts/${service}"
  mkdir -p "$go_out/server" "$go_out/client" "$ts_out"

  echo "→ $service: Go server"
  oapi-codegen -package server -generate types,strict-server,std-http \
    -o "$go_out/server/server.gen.go" "$spec"

  echo "→ $service: Go client"
  oapi-codegen -package client -generate types,client \
    -o "$go_out/client/client.gen.go" "$spec"

  echo "→ $service: TS client"
  openapi-typescript "$spec" --output "$ts_out/index.d.ts"
done
