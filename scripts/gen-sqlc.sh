#!/usr/bin/env bash
# Regenerate sqlc Go code for every service that has a sqlc.yaml (ADR-0007).
set -euo pipefail

shopt -s nullglob

for cfg in services/*/sqlc.yaml; do
  service_dir=$(dirname "$cfg")
  echo "→ sqlc: $service_dir"
  (cd "$service_dir" && sqlc generate)
done
