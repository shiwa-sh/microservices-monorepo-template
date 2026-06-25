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
# Local substitutions vs prod (ADR-0016): Cilium runs alongside kube-proxy;
# object storage is in-cluster MinIO (not an off-cluster bucket); TLS is a
# self-signed wildcard. Secrets use SOPS like every other env — the sops-operator
# decrypts infra/gitops/platform/local/secrets/*.enc.yaml with the committed
# throwaway age key. The data tier (CNPG, the Temporal chart, the SpiceDB chart)
# and observability are the SAME charts staging/prod run, just at instances=1 —
# see infra/gitops/platform/local/values.yaml for the only delta.
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
# 0. Profile (ADR-0016): a thin selector over the SAME charts/values, toggling
#    which platform components come up. The cluster + Cilium + namespace + TLS +
#    SOPS secrets are the always-on baseline; everything else is profile-gated.
#      min      — Postgres only (a service author iterating against a DB)
#      backend  — + Temporal + SpiceDB (workflows + authz)
#      obs      — observability + its MinIO backend (the LGTM/Faro slice)
#      full     — everything, incl. the Ory edge, gateway and services (default)
# ---------------------------------------------------------------------------
PROFILE="${1:-full}"
case "$PROFILE" in
  min)     COMPONENTS="postgres" ;;
  backend) COMPONENTS="postgres temporal spicedb" ;;
  obs)     COMPONENTS="minio observability" ;;
  full)    COMPONENTS="minio postgres temporal spicedb ory observability gateway services" ;;
  *) echo "unknown profile '$PROFILE' — use one of: min | backend | obs | full" >&2; exit 1 ;;
esac
want() { case " $COMPONENTS " in *" $1 "*) return 0 ;; *) return 1 ;; esac; }
echo "→ profile '${PROFILE}': ${COMPONENTS}"

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
    --k3s-arg '--kubelet-arg=eviction-hard=imagefs.available<5%,nodefs.available<5%@server:*' \
    "${PROXY_ARGS[@]}"
fi
kubectl config use-context "k3d-${CLUSTER}"

# When egress goes through the loopback proxy, the node's containerd is pointed at
# http://host.k3d.internal:<port>. k3d only teaches CoreDNS that name (for pods),
# not the node container's /etc/hosts — and Docker rewrites /etc/hosts on every
# container start, so after a host/Docker restart the node can no longer resolve
# the proxy. Image pulls (even the pause sandbox) then fail, Cilium never starts,
# and the whole cluster wedges. Re-assert the entry idempotently on every run.
if [ -n "$host_proxy" ]; then
  for node in $(docker ps --format '{{.Names}}' \
      --filter "label=k3d.cluster=${CLUSTER}" \
      --filter "label=k3d.role=server" --filter "label=k3d.role=agent"); do
    gw="$(docker inspect "$node" --format '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}')"
    docker exec "$node" sh -c \
      "grep -q host.k3d.internal /etc/hosts || echo '${gw} host.k3d.internal' >> /etc/hosts"
  done
fi

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
  --set cilium.operator.replicas=1 \
  --timeout 5m
# Single operator replica locally (default 2 needs 2 nodes for anti-affinity);
# set via Helm so it owns .spec.replicas — an imperative `kubectl scale` would
# grab that field and make the next helm upgrade fail with an SSA conflict.
echo "→ waiting for the node to go Ready (Cilium up)…"
# Cold Cilium image pulls (esp. proxy-routed) can take >5min; give them headroom
# so a from-scratch bring-up doesn't trip on a too-tight node-ready wait.
k wait --for=condition=Ready node --all --timeout=600s

# ---------------------------------------------------------------------------
# 3. Namespace, local TLS, and SOPS-decrypted platform secrets.
# ---------------------------------------------------------------------------
k create namespace "$NS" --dry-run=client -o yaml | k apply -f -

# wildcard-tls is the one local Secret minted imperatively: a self-signed cert
# generated per-machine (its private key is not a shared credential to commit,
# even encrypted, and it rotates on expiry). Every *credential* below comes from
# SOPS instead.
if ! k -n "$NS" get secret wildcard-tls >/dev/null 2>&1; then
  echo "→ generating self-signed wildcard TLS for *.${DOMAIN}"
  tmp="$(mktemp -d)"
  openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
    -keyout "$tmp/tls.key" -out "$tmp/tls.crt" \
    -subj "/CN=*.${DOMAIN}" -addext "subjectAltName=DNS:*.${DOMAIN},DNS:${DOMAIN}" >/dev/null 2>&1
  k -n "$NS" create secret tls wildcard-tls --cert="$tmp/tls.crt" --key="$tmp/tls.key"
  rm -rf "$tmp"
fi

# sops-operator: decrypts the committed SopsSecret into native Secrets — the same
# mechanism dev/staging/prod use (ADR-0005). Its decryption key is the throwaway
# local age key committed at infra/gitops/platform/local/age.key.
echo "→ installing sops-operator + materialising platform secrets"
k -n "$NS" create secret generic sops-age-key \
  --from-file=keys.txt=infra/gitops/platform/local/age.key \
  --dry-run=client -o yaml | k apply -f -
helm dependency update infra/helm/platform/sops-operator >/dev/null
h upgrade --install sops-operator infra/helm/platform/sops-operator -n "$NS" --timeout 5m
k -n "$NS" wait --for=condition=Available deploy \
  -l app.kubernetes.io/name=sops-secrets-operator --timeout=180s || true
# Apply the encrypted SopsSecret; the operator reconciles it into one Secret per
# secretTemplates[] entry. Wait for them so the charts below find their creds.
k apply -f infra/gitops/platform/local/secrets/platform.enc.yaml
for s in observability-bucket postgres-superuser temporal-db-creds spicedb-creds \
         catalog-db orders-db orgs-db payment-db; do
  for _ in $(seq 1 60); do
    k -n "$NS" get secret "$s" >/dev/null 2>&1 && break
    sleep 2
  done
done

# ---------------------------------------------------------------------------
# 4. Real data tier: CNPG (Postgres), the Temporal chart, the SpiceDB chart —
#    the same charts staging/prod run, at instances=1 (ADR-0016). Provisions
#    orders/catalog/orgs/payment/kratos/temporal/temporal_visibility/spicedb
#    as sibling databases via the Cluster's postInitApplicationSQL. Their creds
#    (postgres-superuser, temporal-db-creds, spicedb-creds) come from SOPS above.
# ---------------------------------------------------------------------------
if want postgres; then
  echo "→ installing CNPG (Postgres)"
  helm dependency update infra/helm/platform/postgres >/dev/null
  # The cloudnative-pg subchart ships its CRDs as ordinary templates, so a single
  # install races the Cluster/Pooler CRs against CRD registration (prod gets this
  # ordering from ArgoCD sync-waves). Two passes instead: operator + CRDs first
  # (cluster.create=false), wait for the CRD to register, then add the CRs.
  h upgrade --install postgres infra/helm/platform/postgres -n "$NS" \
    -f infra/gitops/platform/local/values.yaml --set cluster.create=false --timeout 5m
  k wait --for=condition=Established crd/clusters.postgresql.cnpg.io --timeout=120s
  k wait --for=condition=Established crd/poolers.postgresql.cnpg.io --timeout=120s
  # The Cluster CR goes through the operator's mutating webhook, so the operator
  # must be Running (its webhook service endpoints populated) before pass two —
  # not just the CRDs registered. Wait on the deployment.
  k -n "$NS" rollout status deploy/cnpg-operator --timeout=300s
  h upgrade --install postgres infra/helm/platform/postgres -n "$NS" \
    -f infra/gitops/platform/local/values.yaml --timeout 5m
  k -n "$NS" wait --for=condition=Ready cluster/postgres --timeout=300s
  k -n "$NS" wait --for=condition=Ready pod -l cnpg.io/poolerName=postgres-rw --timeout=120s || true
fi

if want temporal; then
  echo "→ installing Temporal"
  helm dependency update infra/helm/platform/temporal >/dev/null
  h upgrade --install temporal infra/helm/platform/temporal -n "$NS" \
    -f infra/gitops/platform/local/values.yaml --timeout 8m
fi

if want spicedb; then
  echo "→ installing SpiceDB"
  h upgrade --install spicedb infra/helm/platform/spicedb -n "$NS" \
    -f infra/gitops/platform/local/values.yaml --timeout 5m
fi

# ---------------------------------------------------------------------------
# 5. MinIO (in-cluster S3). The chart's own bucket-creation job (values.yaml
#    `minio.buckets`) provisions every backend's bucket — no separate mc job.
# ---------------------------------------------------------------------------
if want minio; then
  echo "→ installing MinIO"
  helm dependency update infra/helm/platform/minio >/dev/null
  h upgrade --install minio infra/helm/platform/minio -n "$NS" --timeout 5m || true
  k -n "$NS" rollout status deploy/minio --timeout=180s || true
fi

# ---------------------------------------------------------------------------
# 6. Ory (Kratos + Oathkeeper) — the edge identity stack.
# ---------------------------------------------------------------------------
if want ory; then
  echo "→ installing Ory (Kratos + Oathkeeper)"
  helm dependency update infra/helm/platform/ory >/dev/null
  h upgrade --install ory infra/helm/platform/ory -n "$NS" \
    -f infra/gitops/platform/local/values.yaml --timeout 8m || true
fi

# ---------------------------------------------------------------------------
# 7. Observability (Grafana LGTM monolithic + OTel Collector), MinIO-backed.
# ---------------------------------------------------------------------------
if want observability; then
  echo "→ installing observability"
  # Grafana mounts the `grafana-dashboards` ConfigMap (chart values
  # dashboardsConfigMaps.default). Materialise it from the committed dashboards so
  # the pod can start — prod gets the same ConfigMap from its dashboards source.
  k -n "$NS" create configmap grafana-dashboards \
    --from-file=infra/observability/dashboards/ \
    --dry-run=client -o yaml | k apply -f -
  helm dependency update infra/helm/platform/observability >/dev/null
  h upgrade --install observability infra/helm/platform/observability -n "$NS" \
    -f infra/gitops/platform/local/values.yaml --timeout 10m || true
fi

# ---------------------------------------------------------------------------
# 8. Edge routing: Traefik middlewares + cross-cutting IngressRoutes.
# ---------------------------------------------------------------------------
if want gateway; then
  echo "→ applying gateway (Traefik middlewares + routes)"
  # Traefik opt-ins (cross-namespace + ExternalName) must land before the routes
  # that depend on them; the bundled Traefik redeploys to pick the config up.
  k apply -f infra/local/traefik-config.yaml
  k apply -k infra/gateway
  # Local-only edge: route /auth + landing to the host-run `next dev` (the frontend
  # is not deployed in-cluster locally) and the Kratos public API to ory-kratos.
  k apply -f infra/local/edge-auth.yaml
  # Point frontend-dev at the host (docker bridge gateway) on :3000 — the dev server
  # isn't in-cluster and CoreDNS has no host.k3d.internal record, so wire the
  # EndpointSlice explicitly. Run `next dev -H 0.0.0.0` on the host to serve it.
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
fi

# ---------------------------------------------------------------------------
# 9. Services — build local images and deploy via the same service chart as prod.
# ---------------------------------------------------------------------------
if want services; then
  echo "→ building + deploying services (skaffold, full-tier overlay)"
  # -p full layers infra/helm/values/full-service.yaml so each service is exposed
  # under /api/<name>/ through the edge and ships telemetry (vs the inner loop's
  # direct-call, no-edge default).
  skaffold run -p full --kube-context "k3d-${CLUSTER}" --default-repo="" || true
fi

cat <<EOF

✓ cluster:full up (profile '${PROFILE}': ${COMPONENTS}).
  Edge (Traefik):   https://${DOMAIN}:8443/api/<service>/   (self-signed TLS)
  Hubble UI:        https://hubble.${DOMAIN}:8443/
  Grafana:          https://${DOMAIN}:8443/grafana/   (login admin/admin)
  Teardown:         mise run cluster:full:down

  Profiles:         mise run cluster:full:up [min|backend|obs|full]   (default full)
  Note: on a restricted network where the registry blocks digest pulls, pre-pull
  the platform images by tag and 'k3d image import' them (see docs/dev-loop.md).
EOF
