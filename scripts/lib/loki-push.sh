#!/usr/bin/env bash
# Push one structured line to Loki from outside the cluster (ADR-0104, ADR-0500).
#
#   source scripts/lib/loki-push.sh
#   loki_push '{"service":"catalog","env":"dev"}' '{"digest":"sha256:…"}'
#
# WHY A DIRECT PUSH AND NOT THE COLLECTOR. Every in-cluster signal reaches the
# backends through the node-level OTel collector, which is ADR-0500's invariant and
# is not being broken here: this runs in CI, which is not in the cluster and has no
# collector to push to. The record is a supply-chain fact about a build, produced by
# the machine that produced the build, and the only place it can come from is there.
#
# LABELS ARE THE CALLER'S, AND THEY ARE FEW ON PURPOSE. Loki builds one stream per
# distinct label set, so a label whose value changes every build — a digest, a commit
# — mints a stream per build and turns the index into a list of things that happened
# once. Those belong in the LINE, which is why this takes the two separately and the
# callers label by `service` and `env` only.
#
# A MISSING ENDPOINT IS A WARNING, NOT A FAILURE. LOKI_PUSH_URL is unset until a
# project has somewhere to push to, and a signed, attested image that could not file
# its paperwork is still a signed, attested image — failing the publish would trade a
# real control for a reporting one. The absence is loud in the log and visible as a
# gap on the supply-chain dashboard, which is where someone is looking for it.

# loki_push <stream-labels-json> <line-json>
loki_push() {
  local labels="$1" line="$2"

  if [ -z "${LOKI_PUSH_URL:-}" ]; then
    warn "LOKI_PUSH_URL unset — supply-chain record not filed (this build is still signed and attested)"
    return 0
  fi

  # Nanoseconds since the epoch, as a string: Loki's push API takes the timestamp as
  # a decimal string and rejects a JSON number for it.
  local ns
  ns="$(date +%s%N)"

  local payload
  payload="$(jq -cn --argjson stream "$labels" --arg ns "$ns" --arg line "$line" \
    '{streams: [{stream: $stream, values: [[$ns, $line]]}]}')"

  # --fail so a 4xx is an error rather than a body printed and ignored; the whole
  # call is still tolerated by the caller, but a broken push should say so.
  if curl --silent --show-error --fail --max-time 20 \
    -H "Content-Type: application/json" \
    ${LOKI_PUSH_TENANT:+-H "X-Scope-OrgID: ${LOKI_PUSH_TENANT}"} \
    ${LOKI_PUSH_AUTH:+-H "Authorization: ${LOKI_PUSH_AUTH}"} \
    -X POST "${LOKI_PUSH_URL%/}/loki/api/v1/push" \
    --data-binary "$payload" >/dev/null; then
    detail "supply-chain record filed to Loki"
  else
    warn "supply-chain record could NOT be filed to Loki (the build itself is unaffected)"
  fi
}
