#!/usr/bin/env bash
# Regenerate Go servers/clients and TS clients from every service's OpenAPI spec.
# Outputs land in libs/{go,ts}/sdks/<service>/ (ADR-0303).
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
  # Regenerate in place: `ogen --clean` overwrites the generated files and prunes
  # orphans itself, so no `rm -rf` is needed. Skipping it keeps the package dir
  # populated throughout — a concurrent `go`/lint pass never observes an empty
  # dir mid-run (which fails as "no Go files"/"package not found"). Use `gen:clean`
  # to wipe and repopulate from scratch (e.g. after deleting a service).
  mkdir -p "$go_out"
  ogen --target "$go_out" --package "$pkg" --clean "$spec"

  echo "→ $service: TS client"
  mkdir -p "$ts_out"

  bun x openapi-typescript@7.13.0 "$spec" --output "$ts_out/index.d.ts"

  # A package.json beside it, so the generated SDK is a real workspace package
  # rather than a bare .d.ts reachable only through a tsconfig path alias. The
  # alias alone made every import of it an UNDECLARED dependency — which Biome
  # rejects, so callers hand-wrote a second copy of the response type instead of
  # importing the generated one. A copy of a contract is the thing this whole
  # generator exists to remove.
  cat >"$ts_out/package.json" <<JSON
{
  "name": "@sdks/${service}",
  "version": "0.0.0",
  "private": true,
  "description": "Generated TypeScript client types for ${service} (ADR-0303). Do not edit.",
  "type": "module",
  "exports": {
    ".": "./index.d.ts"
  },
  "types": "./index.d.ts"
}
JSON
done

# The lockfile, because the block above just wrote workspace manifests. Each
# generated SDK is a workspace package (`libs/ts/sdks/*` is in the root
# `workspaces` glob), so adding a service adds a package — and CI installs with
# `--frozen-lockfile`, which rejects a lockfile that does not already know about
# it. Without this, generating a spec produces a tree that builds locally and
# fails on the first clean install.
bun install >/dev/null 2>&1 || true
