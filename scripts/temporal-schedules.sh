#!/usr/bin/env bash
# Apply the committed Temporal Schedules (ADR-0302).
#
#   mise run temporal:schedules
#
# Idempotent: an existing schedule is updated rather than duplicated, so this is
# safe to run on every deploy. That matters more than it sounds — a schedule
# created twice fires twice, and a retention pass that runs twice concurrently is
# two workflows deleting the same rows.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

SPEC="infra/temporal/schedules.yaml"
ADDRESS="${TEMPORAL_HOST_PORT:-localhost:7233}"
NAMESPACE="${TEMPORAL_NAMESPACE:-default}"

step "applying schedules from ${SPEC} to ${ADDRESS}"

count=$(yq -r '.schedules | length' "$SPEC")
for i in $(seq 0 $((count - 1))); do
  id=$(yq -r ".schedules[$i].id" "$SPEC")
  workflow=$(yq -r ".schedules[$i].workflow" "$SPEC")
  queue=$(yq -r ".schedules[$i].taskQueue" "$SPEC")
  cron=$(yq -r ".schedules[$i].cron" "$SPEC")

  if temporal schedule describe --schedule-id "$id" --address "$ADDRESS" --namespace "$NAMESPACE" >/dev/null 2>&1; then
    temporal schedule update --schedule-id "$id" --address "$ADDRESS" --namespace "$NAMESPACE" \
      --workflow-type "$workflow" --task-queue "$queue" --cron "$cron" \
      --workflow-id "${id}-run" >/dev/null
    detail "updated ${id}"
  else
    temporal schedule create --schedule-id "$id" --address "$ADDRESS" --namespace "$NAMESPACE" \
      --workflow-type "$workflow" --task-queue "$queue" --cron "$cron" \
      --workflow-id "${id}-run" >/dev/null
    detail "created ${id}"
  fi
done

ok "${count} schedules applied"
