#!/usr/bin/env bash
# Full local platform — the heavy counterpart to `cluster:up` (ADR-0003).
#
# `cluster:up` gives the inner dev loop (k3d + lightweight Postgres/Temporal/
# SpiceDB) and intentionally omits the platform. This script brings the platform
# up locally so the EDGE (Traefik + Ory Oathkeeper, ADR-0009), the real CNI
# (Cilium → NetworkPolicy + Hubble, ADR-0003), Kratos, SpiceDB and a MinIO-backed
# Grafana LGTM stack (ADR-0011) can be exercised end to end on a laptop.
#
# It installs charts directly with `helm` — NOT ArgoCD. ArgoCD reconciles from
# git@master (infra/gitops), so it would deploy committed master, not your tree;
# direct helm is the documented local alternative (docs/dev-loop.md).
#
# Requirements: ~16GB free RAM (32GB comfortable), Docker, and the mise tools.
# Local substitutions vs prod: Cilium runs alongside kube-proxy; object storage
# is in-cluster MinIO (not an off-cluster bucket); TLS is a self-signed wildcard;
# secrets are plain (not SOPS); data deps are the lightweight ones, not CNPG /
# the Temporal Helm chart (those stay staging/prod — too heavy for a laptop).
#
# Teardown: mise run cluster:full:down
set -euo pipefail

CLUSTER="platform-full"
NS="platform"
# *.localtest.me resolves to 127.0.0.1, so no /etc/hosts edits are needed.
DOMAIN="${DOMAIN:-dev.localtest.me}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }
h() { helm --kube-context "k3d-${CLUSTER}" "$@"; }

# ---------------------------------------------------------------------------
# 1. k3d cluster with Cilium as the CNI (flannel + built-in netpol disabled).
#    Traefik stays (it provides the IngressRoute/Middleware CRDs the edge uses).
# ---------------------------------------------------------------------------
# If the host routes egress through a loopback HTTP proxy (some sandboxes do, and
# some registries 403 on digest pulls without it), point the node's containerd at
# it via host.k3d.internal so in-cluster pulls — including ArgoCD-synced workloads
# — go through it. No proxy on the host → these stay empty (normal laptop path).
PROXY_ARGS=()
host_proxy="${HTTPS_PROXY:-${https_proxy:-}}"
if [ -n "$host_proxy" ]; then
  node_proxy="$(printf '%s' "$host_proxy" | sed -E 's#//(127\.0\.0\.1|localhost)#//host.k3d.internal#')"
  no_proxy="10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.svc,.svc.cluster.local,cluster.local,127.0.0.1,localhost,host.k3d.internal,.localtest.me"
  echo "→ routing node image pulls through proxy ${node_proxy}"
  PROXY_ARGS=(
    --env "HTTP_PROXY=${node_proxy}@server:*"
    --env "HTTPS_PROXY=${node_proxy}@server:*"
    --env "NO_PROXY=${no_proxy}@server:*"
  )
fi

if ! k3d cluster list 2>/dev/null | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ creating k3d cluster '$CLUSTER'"
  k3d cluster create "$CLUSTER" \
    --servers 1 --agents 0 \
    --port "8080:80@loadbalancer" --port "8443:443@loadbalancer" \
    --k3s-arg '--flannel-backend=none@server:*' \
    --k3s-arg '--disable-network-policy@server:*' \
    "${PROXY_ARGS[@]}"
fi
kubectl config use-context "k3d-${CLUSTER}"

# ---------------------------------------------------------------------------
# 2. Cilium. kubeProxyReplacement is off for local reliability (k3d keeps
#    kube-proxy); NetworkPolicy + Hubble work regardless.
# ---------------------------------------------------------------------------
SERVER_IP="$(k get node -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
echo "→ installing Cilium (apiserver ${SERVER_IP}:6443)"
helm dependency update infra/helm/platform/cilium >/dev/null
h upgrade --install cilium infra/helm/platform/cilium -n kube-system \
  --set cilium.kubeProxyReplacement=false \
  --set cilium.k8sServiceHost="${SERVER_IP}" \
  --set cilium.k8sServicePort=6443 \
  --timeout 5m
# Single operator replica locally (default 2 needs 2 nodes for anti-affinity).
k -n kube-system scale deploy/cilium-operator --replicas=1
echo "→ waiting for the node to go Ready (Cilium up)…"
k wait --for=condition=Ready node --all --timeout=300s

# ---------------------------------------------------------------------------
# 3. Namespace, local TLS, and stub secrets (SOPS is bypassed locally).
# ---------------------------------------------------------------------------
k create namespace "$NS" --dry-run=client -o yaml | k apply -f -

if ! k -n "$NS" get secret wildcard-tls >/dev/null 2>&1; then
  echo "→ generating self-signed wildcard TLS for *.${DOMAIN}"
  tmp="$(mktemp -d)"
  openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
    -keyout "$tmp/tls.key" -out "$tmp/tls.crt" \
    -subj "/CN=*.${DOMAIN}" -addext "subjectAltName=DNS:*.${DOMAIN},DNS:${DOMAIN}" >/dev/null 2>&1
  k -n "$NS" create secret tls wildcard-tls --cert="$tmp/tls.crt" --key="$tmp/tls.key"
  rm -rf "$tmp"
fi

# MinIO credentials, reused as the S3 creds Loki/Mimir/Tempo read.
k -n "$NS" create secret generic observability-bucket \
  --from-literal=AWS_ACCESS_KEY_ID=minioadmin \
  --from-literal=AWS_SECRET_ACCESS_KEY=minioadmin \
  --dry-run=client -o yaml | k apply -f -

# Per-service DATABASE_URL secrets (point at the lightweight Postgres).
for svc in catalog orders orgs payment; do
  k -n "$NS" create secret generic "${svc}-db" \
    --from-literal=DATABASE_URL="postgres://dev:dev@postgres.${NS}.svc.cluster.local:5432/${svc}?sslmode=disable" \
    --dry-run=client -o yaml | k apply -f -
done
k -n "$NS" create secret generic spicedb-creds \
  --from-literal=SPICEDB_PRESHARED_KEY=localdevkey --dry-run=client -o yaml | k apply -f -

# ---------------------------------------------------------------------------
# 4. Lightweight data deps (Postgres / Temporal dev / SpiceDB) + Kratos DB.
# ---------------------------------------------------------------------------
echo "→ applying lightweight data deps"
k apply -f infra/local/deps.yaml
k -n "$NS" rollout status deploy/postgres --timeout=180s
k -n "$NS" exec deploy/postgres -- psql -U dev -c "CREATE DATABASE kratos;" 2>/dev/null || true

# ---------------------------------------------------------------------------
# 5. MinIO (in-cluster S3) + buckets for the LGTM backends.
# ---------------------------------------------------------------------------
echo "→ installing MinIO"
h upgrade --install minio infra/helm/platform/minio -n "$NS" --timeout 5m || true
k -n "$NS" rollout status deploy/minio --timeout=180s || true
echo "→ creating buckets"
k -n "$NS" delete job minio-mkbucket --ignore-not-found >/dev/null 2>&1 || true
cat <<'EOF' | k apply -f -
apiVersion: batch/v1
kind: Job
metadata: { name: minio-mkbucket, namespace: platform }
spec:
  backoffLimit: 3
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: mc
          image: minio/mc:latest
          command: ["/bin/sh","-c"]
          args:
            - |
              mc alias set m http://minio.platform.svc.cluster.local:9000 minioadmin minioadmin &&
              for b in loki-chunks loki-ruler loki-admin mimir-blocks mimir-ruler mimir-alertmanager tempo-traces; do
                mc mb -p m/$b || true
              done
EOF

# ---------------------------------------------------------------------------
# 6. Ory (Kratos + Oathkeeper) — the edge identity stack.
# ---------------------------------------------------------------------------
echo "→ installing Ory (Kratos + Oathkeeper)"
helm dependency update infra/helm/platform/ory >/dev/null
h upgrade --install ory infra/helm/platform/ory -n "$NS" \
  -f infra/helm/values/local-platform.yaml --timeout 8m || true

# ---------------------------------------------------------------------------
# 7. Observability (Grafana LGTM monolithic + OTel Collector), MinIO-backed.
# ---------------------------------------------------------------------------
echo "→ installing observability"
helm dependency update infra/helm/platform/observability >/dev/null
h upgrade --install observability infra/helm/platform/observability -n "$NS" \
  -f infra/helm/values/local-platform.yaml --timeout 10m || true

# ---------------------------------------------------------------------------
# 8. Edge routing: Traefik middlewares + cross-cutting IngressRoutes.
# ---------------------------------------------------------------------------
echo "→ applying gateway (Traefik middlewares + routes)"
k apply -k infra/gateway

# ---------------------------------------------------------------------------
# 9. Services — build local images and deploy via the same service chart as prod.
# ---------------------------------------------------------------------------
echo "→ building + deploying services (skaffold)"
skaffold run --kube-context "k3d-${CLUSTER}" --default-repo="" || true

cat <<EOF

✓ cluster:full up.
  Edge (Traefik):   https://${DOMAIN}:8443/api/<service>/   (self-signed TLS)
  Hubble UI:        https://${DOMAIN}:8443/hubble/
  Grafana:          kubectl -n ${NS} port-forward svc/grafana 3000:80
  Teardown:         mise run cluster:full:down

  Note: on a restricted network where the registry blocks digest pulls, pre-pull
  the platform images by tag and 'k3d image import' them (see docs/dev-loop.md).
EOF
