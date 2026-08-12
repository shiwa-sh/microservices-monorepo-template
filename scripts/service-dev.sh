#!/usr/bin/env bash
# Run a service NATIVELY behind the real edge (ADR-0205, ADR-0600) — the other half
# of `cluster:add`, and the reason the identity stack is in the local floor.
#
#   mise run service:dev -- orders
#
# Stamps the per-service edge glue so Traefik routes `/api/<resource>` to the host
# process: a selector-less Service, an EndpointSlice at the docker-bridge gateway,
# and an IngressRoute carrying the SAME middleware chain the deployed service gets.
# That chain is the point — a native service reached this way is handed real
# Oathkeeper identity headers, where one curled directly on :8080 is handed forged
# ones, and the difference is exactly the class of bug that otherwise only appears
# after deploy.
#
# The glue is per-machine state (the bridge IP is discovered at runtime and moves
# across restarts), so it is never GitOps-managed — same reasoning as the frontend
# catch-all in infra/local/edge-auth.yaml.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/ports.sh"

CLUSTER="${CLUSTER:-platform}"
NS="platform"
DOMAIN="${DOMAIN:-dev.localtest.me}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SVC="${1:?usage: mise run service:dev -- <svc>}"
# The registry, not a flag: the glue must point at the same port the service's own
# .mise.toml makes it bind, or Traefik forwards to nothing. One source of truth.
PORT="$(service_port "$SVC")"
SVC_DIR="services/${SVC}"
VALUES="infra/gitops/services/local/values/${SVC}.yaml"

[ -d "$SVC_DIR" ] || fail "no such service: ${SVC_DIR}"
[ -f "$VALUES" ] || fail "missing local values: ${VALUES}"

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }

# The deployed service and the native one would both answer for the same routes.
# Rather than racing them on route priority, refuse — one of them is what you meant.
if k -n "$NS" get "deploy/${SVC}-server" >/dev/null 2>&1; then
  fail "${SVC} is deployed in-cluster; run 'mise run cluster:remove -- ${SVC}' first"
fi

step "stamping the edge glue for a native ${SVC}"

# Selector-less Service; the EndpointSlice below supplies the address.
k apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: ${SVC}-dev
  namespace: ${NS}
  labels: { local.platform/glue: "${SVC}" }
spec:
  ports:
    - name: http
      port: ${PORT}
      targetPort: ${PORT}
EOF

# The same resources and middleware chain the service chart routes when deployed,
# read from the committed local values so the two paths cannot drift.
resources="$(yq -r '.ingress.resources // [] | join(" ")' "$VALUES")"
[ -n "$resources" ] || fail "${SVC} declares no ingress.resources in ${VALUES} (nothing to route)"

match=""
for r in $resources; do
  [ -n "$match" ] && match="${match} || "
  match="${match}PathPrefix(\`/api/${r}\`)"
done

# Mirrors infra/helm/service/templates/ingressroute.yaml exactly — same match
# (service.ingressMatch), same middleware chain, same order. Flat-API routing
# (ADR-0306) means /api is stripped, so the native process mounts its resources at
# root just as the deployed pod does.
k apply -f - <<EOF
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: ${SVC}-dev-stripprefix
  namespace: ${NS}
  labels: { local.platform/glue: "${SVC}" }
spec:
  stripPrefix:
    prefixes:
      - /api
---
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: ${SVC}-dev
  namespace: ${NS}
  labels: { local.platform/glue: "${SVC}" }
spec:
  entryPoints: [websecure]
  routes:
    - kind: Rule
      match: Host(\`${DOMAIN}\`) && (${match})
      middlewares:
        # Anti-spoofing (ADR-0305): strip client-supplied identity headers BEFORE
        # forwardAuth, so nothing can inject X-User-* on an anonymous /api route.
        - name: strip-identity-headers
        - name: oathkeeper-forward-auth
        - name: security-headers
        - name: ${SVC}-dev-stripprefix
      services:
        - name: ${SVC}-dev
          port: ${PORT}
  tls:
    secretName: wildcard-tls
EOF

bash scripts/cluster-edge-glue.sh "$SVC" "$PORT"

cat <<EOF

✓ ${SVC} glued to the host on :${PORT}.

  Now run it (its own tasks pull the dependency components in):
    cd services/${SVC} && mise run server

  Reached at:  https://${DOMAIN}:8443/api/$(echo "$resources" | cut -d' ' -f1)
  Through:     the real Oathkeeper chain — identity headers are genuine here
  Remove:      mise run cluster:remove -- ${SVC}
EOF
