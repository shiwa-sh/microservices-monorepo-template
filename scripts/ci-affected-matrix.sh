#!/usr/bin/env bash
# Emit the affected manifest as forge step-output assignments (ADR-0102). The
# matrix jobs in publish.yml need `services` and `apps` as separate JSON arrays;
# splitting the manifest is pipeline logic, so it lives here and the workflow step
# only redirects into $GITHUB_OUTPUT.
set -euo pipefail

manifest="$(mise run ci:affected)"
printf 'services=%s\n' "$(printf '%s' "$manifest" | jq -c '.services')"
printf 'apps=%s\n' "$(printf '%s' "$manifest" | jq -c '.apps')"

# The image matrix as explicit {service, cmd} PAIRS, not a cross product of
# services and [server, worker]. Not every service has a worker — catalog owns no
# process — and a cross product asks the builder for `cmd/worker` in a service that
# has no such directory, which fails the build for that one cell. `fail-fast: false`
# keeps the other images publishing, so the symptom is a workflow that is red on
# every run while every image it was supposed to produce exists.
#
# The filesystem is the source of truth here rather than a list in this script: a
# service gains a worker by gaining the directory, and a list would be the copy
# that goes stale.
images='[]'
for svc in $(printf '%s' "$manifest" | jq -r '.services[]'); do
  for cmd in server worker; do
    [ -d "services/${svc}/cmd/${cmd}" ] || continue
    images="$(printf '%s' "$images" | jq -c --arg s "$svc" --arg c "$cmd" '. + [{service: $s, cmd: $c}]')"
  done
done
printf 'images=%s\n' "$images"
