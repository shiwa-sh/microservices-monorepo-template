#!/usr/bin/env bash
# The local port registry gate (ADR-0205). Two services quietly sharing a port is a
# bind race that only surfaces when someone runs both — which is exactly the
# multi-service debugging case the registry exists to make possible. So it is
# checked here rather than discovered there.
#
# Three invariants:
#   1. Registered ports are unique.
#   2. Every service has an entry.
#   3. Each service's .mise.toml binds the port the registry assigns it. The two
#      have to agree: svc-apply.sh forwards to the registry's port and the service
#      binds .mise.toml's, so a drift between them fails at runtime as a connection
#      refused with no hint of why.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/ports.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

rc=0

dupes="$(all_port_entries | cut -d: -f2 | sort | uniq -d)"
if [ -n "$dupes" ]; then
  for p in $dupes; do
    warn "port ${p} is registered to more than one service: $(all_port_entries | grep ":${p}\$" | cut -d: -f1 | tr '\n' ' ')"
  done
  rc=1
fi

# 8080 must stay unassigned: the local edge maps host 8080, so a service bound
# there would shadow it.
if all_port_entries | grep -q ':8080$'; then
  warn "8080 is reserved for the local edge mapping — pick another port"
  rc=1
fi

for dir in services/*/; do
  svc="$(basename "$dir")"
  [ "${svc#_}" = "$svc" ] || continue # skip _template

  if ! registered="$(service_port "$svc" 2>/dev/null)"; then
    warn "${svc} has no entry in scripts/lib/ports.sh"
    rc=1
    continue
  fi

  declared="$(grep -oP '^PORT\s*=\s*"\K[0-9]+' "${dir}.mise.toml" || true)"
  if [ -z "$declared" ]; then
    warn "${svc}/.mise.toml sets no PORT — it would bind :8080 and ignore the registry"
    rc=1
  elif [ "$declared" != "$registered" ]; then
    warn "${svc}: registry says ${registered}, ${svc}/.mise.toml binds ${declared}"
    rc=1
  fi
done

[ "$rc" -eq 0 ] && ok "local port registry consistent"
exit "$rc"
