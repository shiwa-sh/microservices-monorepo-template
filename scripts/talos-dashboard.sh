#!/usr/bin/env bash
# The Talos node dashboard for the local tier (ADR-0200).
#
#   mise run talos:dashboard              # every node
#   mise run talos:dashboard -- 10.5.0.2  # one
#
# Talos ships no web UI, and this is what exists instead: a terminal dashboard over
# the node API showing service state, logs and live resource use. The same view is
# on each node's console, which is where a node that has not joined the network yet
# can still be read.
#
# Node addresses come from the provisioner's subnet rather than from kubectl: the
# question this answers is often "why is that node not in kubectl", so it must not
# depend on the API server being up.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

SUBNET="${TALOS_SUBNET:-10.5.0.0/24}"
PREFIX="${SUBNET%.*/*}"
WORKERS="${TALOS_WORKERS:-2}"

if [ "$#" -gt 0 ]; then
  nodes="$*"
else
  # .2 is the control plane; workers follow it.
  nodes="${PREFIX}.2"
  for i in $(seq 1 "$WORKERS"); do
    nodes="${nodes},${PREFIX}.$((2 + i))"
  done
fi

[ -f "$HOME/.talos/config" ] || fail "no talosconfig at ~/.talos/config — bring the cluster up first:
    mise run cluster:full"

detail "nodes: ${nodes}  (q to quit)"
exec talosctl --talosconfig "$HOME/.talos/config" -n "${nodes// /,}" dashboard
