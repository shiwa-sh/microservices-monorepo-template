#!/usr/bin/env bash
# The service contract gate (ADR-0205). Every service under services/ must provide
# the same set of artifacts, because the platform DISCOVERS services through those
# artifacts rather than through a registry:
#
#   - the ApplicationSet generates one Argo Application per values file, so a
#     service with no values file for an environment is simply ABSENT there, with
#     nothing to notice it (this is not hypothetical — the authz authorizer was
#     missing from dev exactly this way while Oathkeeper called it by DNS);
#   - svc-apply.sh and service-dev.sh resolve a service's local port from the
#     registry, so an unregistered service cannot be run natively or called;
#   - service-deploy.sh reads dep:*/svc:* out of .mise.toml, so an undeclared
#     dependency is an undeployed dependency.
#
# The contract is therefore mechanical, and so is this check. Whether the
# declarations are TRUE — that a service listing dep:temporal really uses Temporal,
# and that one calling payment says so — is the other half, and it is now
# lint:service-deps, which reads each entrypoint's package closure.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/ports.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

rc=0
checked=0

# Environments are the values directories that actually exist. staging/ and prod/
# are absent by design while the template has no deployed envs; adding one makes it
# required for every service on the next run, which is the intent.
envs=()
for d in infra/gitops/services/*/; do envs+=("$(basename "$d")"); done

# A service may opt out of an environment ON PURPOSE, but it has to say so, in the
# file the opt-out affects. Silence is the failure mode this check exists to remove.
opted_out() { grep -qs "platform/not-deployed: *${2}" "services/${1}/.mise.toml"; }

for dir in services/*/; do
  svc="$(basename "$dir")"
  [ "${svc#_}" = "$svc" ] || continue # _template is the source, not a service
  checked=$((checked + 1))

  # ── Files every service owns ────────────────────────────────────────────────
  for f in .mise.toml .env.example Dockerfile README.md; do
    [ -f "${dir}${f}" ] || {
      warn "${svc}: missing ${f}"
      rc=1
    }
  done

  # ── Standard task names (ADR-0101) ──────────────────────────────────────────
  #
  # `server` only for a service that has one. A worker-only deployable declares
  # `worker` instead, and demanding both would force it to ship a task that runs
  # nothing — which is worse than the gap, because a task that exists is a task
  # someone will run.
  tasks="test lint build"
  if [ -d "services/${svc}/cmd/server" ]; then
    tasks="server ${tasks}"
  else
    tasks="worker ${tasks}"
  fi
  for t in ${tasks}; do
    grep -q "^\[tasks\.${t}\]" "${dir}.mise.toml" 2>/dev/null || {
      warn "${svc}: .mise.toml declares no [tasks.${t}]"
      rc=1
    }
  done

  # ── Isolated from its siblings by depguard (ADR-0101) ───────────────────────
  # The constraint is relational — services/X may not import services/Y — and
  # depguard's unit is a file pattern with a deny list, so it takes one rule per
  # service. A new service that does not add its block is not caught by depguard
  # (it has no rule to break), which is exactly the silence this check removes.
  # Go's own internal/ convention covers today's layout; the rule covers the day a
  # service exports a package outside internal/.
  grep -q "service-isolation-${svc}:" .golangci.yml || {
    warn "${svc}: no 'service-isolation-${svc}' depguard rule in .golangci.yml — nothing stops a sibling importing it"
    rc=1
  }

  # ── Registered local port, agreeing with what the service binds ─────────────
  # (uniqueness and the reverse direction are lint:ports' job)
  #
  # Only for a service that binds one. A worker-only deployable — the platform
  # worker is the first — answers no requests, so demanding a port would be
  # demanding a number nothing listens on, and the registry's value is that every
  # entry in it is real.
  if [ -d "services/${svc}/cmd/server" ]; then
    service_port "$svc" >/dev/null 2>&1 || {
      warn "${svc}: no local port in scripts/lib/ports.sh"
      rc=1
    }
  fi

  # ── Deployable in every environment, or explicitly not ──────────────────────
  for env in "${envs[@]}"; do
    if [ -f "infra/gitops/services/${env}/values/${svc}.yaml" ]; then continue; fi
    if opted_out "$svc" "$env"; then
      detail "${svc}: not deployed to ${env} (declared)"
      continue
    fi
    warn "${svc}: no infra/gitops/services/${env}/values/${svc}.yaml — the ApplicationSet will not deploy it to ${env}, silently. Add the file, or declare '# platform/not-deployed: ${env}' in services/${svc}/.mise.toml"
    rc=1
  done
done

[ "$rc" -eq 0 ] && ok "service contract satisfied (${checked} services)"
exit "$rc"
