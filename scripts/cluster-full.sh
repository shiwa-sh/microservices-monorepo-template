#!/usr/bin/env bash
# Full local platform via ArgoCD (ADR-0004, ADR-0016) — backs `mise run cluster:full`.
# The heavy lifting
# (CRD→operator→instance ordering, secret materialisation, sync waves) is ArgoCD's
# job, the same tool prod runs; this script only does what Argo cannot bootstrap:
# create the cluster, install the CNI + Argo itself, plant the SOPS decryption key,
# apply the local root-app, then wait and wire the host-specific edge tail.
#
# Baseline (choice a): Argo syncs committed master from GitHub — CI-built images,
# identical to prod. To iterate on uncommitted code/infra, see service:deploy /
# platform:deploy, or point the local root-app targetRevision at a pushed branch.
#
# Teardown: mise run cluster:stop (stop) / mise run cluster:delete (delete).
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
NS="platform"
DOMAIN="${DOMAIN:-dev.localtest.me}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }
h() { helm --kube-context "k3d-${CLUSTER}" "$@"; }

# 1. Cluster + CNI (a CNI must exist before Argo's pods can schedule).
bash scripts/cluster-ensure.sh
bash scripts/cilium-install.sh

# 2. ArgoCD (it cannot sync itself into existence). Excluded from the local
#    platform ApplicationSet, so this imperative release is authoritative.
echo "→ installing ArgoCD"
helm dependency update infra/helm/platform/argocd >/dev/null
h upgrade --install argocd infra/helm/platform/argocd -n argocd --create-namespace --timeout 8m
k -n argocd rollout status deploy/argocd-server --timeout=300s
k -n argocd rollout status deploy/argocd-repo-server --timeout=300s
k -n argocd rollout status deploy/argocd-applicationset-controller --timeout=300s

# 3. SOPS decryption key (the bootstrap root of trust): the committed throwaway
#    local age key, planted as the Secret the sops-operator mounts (ADR-0005).
echo "→ planting sops-age-key (local throwaway key)"
k create namespace "$NS" --dry-run=client -o yaml | k apply -f -
k -n "$NS" create secret generic sops-age-key \
  --from-file=keys.txt=infra/gitops/platform/local/age.key \
  --dry-run=client -o yaml | k apply -f -

# 4. Local root App-of-Apps → Argo discovers the local appsets + apps from git.
echo "→ applying local root application"
k apply -f infra/gitops/bootstrap-local/root-application-local.yaml

# 5. Wait for Argo to converge. Generated apps appear asynchronously (the appsets
#    must reconcile first), so poll until every app is Synced + Healthy.
echo "→ waiting for ArgoCD to converge (this is a full platform; first run is slow)…"
deadline=$(( $(date +%s) + 1800 ))
while :; do
  # No apps yet (appsets still generating)? keep waiting.
  total=$(k -n argocd get applications.argoproj.io -o name 2>/dev/null | wc -l)
  if [ "$total" -gt 0 ]; then
    notdone=$(k -n argocd get applications.argoproj.io \
      -o jsonpath='{range .items[*]}{.metadata.name} {.status.sync.status}/{.status.health.status}{"\n"}{end}' 2>/dev/null \
      | grep -vE ' Synced/Healthy$' || true)
    if [ -z "$notdone" ]; then
      echo "✓ all ${total} ArgoCD applications Synced + Healthy"
      break
    fi
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "✗ timed out waiting for convergence; current state:" >&2
    k -n argocd get applications.argoproj.io \
      -o custom-columns=NAME:.metadata.name,SYNC:.status.sync.status,HEALTH:.status.health.status 2>/dev/null >&2 || true
    exit 1
  fi
  sleep 15
done

# 6. Host-specific edge tail (cannot be GitOps — depends on per-machine state):
#    local Traefik tuning, the /auth + landing routes to a host-run frontend, and
#    the frontend-dev EndpointSlice pointing at the docker-bridge gateway IP.
echo "→ applying host-specific edge glue"
k apply -f infra/local/traefik-config.yaml
k apply -f infra/local/edge-auth.yaml
GW="$(docker inspect "k3d-${CLUSTER}-server-0" \
  --format '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}')"
k apply -f - <<EOF
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: frontend-dev
  namespace: ${NS}
  labels:
    kubernetes.io/service-name: frontend-dev
addressType: IPv4
ports:
  - name: http
    port: 3000
    protocol: TCP
endpoints:
  - addresses: ["${GW}"]
    conditions: { ready: true }
EOF

cat <<EOF

✓ cluster:full up (ArgoCD-driven from master).
  Product (Traefik):  https://${DOMAIN}:8443/api/<service>/   (self-signed TLS)
  Ops tier (ADR-0017, one origin per tool, AAL2 operator session required):
    Grafana:          https://grafana.ops.${DOMAIN}:8443/
    Hubble UI:        https://hubble.ops.${DOMAIN}:8443/
    Temporal UI:      https://temporal.ops.${DOMAIN}:8443/
    MinIO console:    https://minio.ops.${DOMAIN}:8443/
    Lowdefy console:  https://console.ops.${DOMAIN}:8443/
  Frontend:           run it natively on :3000 (the frontend-dev EndpointSlice
                      routes /auth + landing to the host).
  Teardown:           mise run cluster:stop  (keep cache) / cluster:delete (delete)
EOF
