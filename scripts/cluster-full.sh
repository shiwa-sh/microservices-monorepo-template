#!/usr/bin/env bash
# Full local platform via ArgoCD (ADR-0201, ADR-0205) — backs `mise run cluster:full`.
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

# The full tier is its own cluster (ADR-0600). Set BEFORE the context library is
# sourced: it resolves the cluster name and kube context from this, and a
# cluster:full that resolved to the inner loop's context would install the whole
# platform onto the wrong cluster.
export TIER=full

CLUSTER="${CLUSTER:-platform}"
NS="platform"
DOMAIN="${DOMAIN:-dev.localtest.me}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

source "$(dirname "$0")/lib/cluster-ctx.sh"

k() { kubectl --context "$(cluster_ctx)" "$@"; }
h() { helm --kube-context "$(cluster_ctx)" "$@"; }
# argocd CLI in core mode: talks straight to the Application CRDs (no `argocd
# login` / argocd-server, ADR-0201). Core mode derives the install namespace from
# the kube-context, so it runs against a throwaway kubeconfig pinned to `argocd`
# (set up in step 5) rather than mutating the user's real context.
ac() { KUBECONFIG="$AC_KUBECONFIG" argocd --core "$@"; }

# 1. Cluster + CNI (a CNI must exist before Argo's pods can schedule).
#
# The cluster is created without one — `disableDefaultCNI` in the tier's kind config
# — so the nodes stay NotReady until Cilium is installed, and nothing schedules
# before it. Both steps are the inner loop's, run against this tier's cluster.
bash scripts/cluster-dispatch.sh ensure
bash scripts/cilium-install.sh

# 1b. Kyverno's admission webhook, on a cluster that already has one.
#
#     Step 1 re-installs Cilium on every run, which cycles the pod network and
#     restarts the Kyverno pods with it. Their ValidatingWebhookConfiguration is
#     `failurePolicy: Fail`, so while the admission controller is unready EVERY
#     apply in the cluster is rejected — and the next one here is ArgoCD's
#     pre-upgrade hook Job. The whole `helm upgrade` then fails on
#     `failed calling webhook "validate.kyverno.svc-fail": connect: connection
#     refused`, which names a webhook and a port and nothing that suggests waiting.
#
#     A first bring-up has no kyverno namespace and skips this; Argo installs it
#     later, in its own wave.
if k get ns kyverno >/dev/null 2>&1; then
  echo "→ waiting for the Kyverno admission webhook to serve"
  k -n kyverno rollout status deploy/kyverno-admission-controller --timeout=300s
  # The rollout is the DEPLOYMENT's view. The webhook is reachable only once the
  # Service has a ready endpoint behind it, which lags the pod going Ready — and it
  # is the endpoint the API server dials.
  for _ in $(seq 1 60); do
    [ -n "$(k -n kyverno get endpointslice -l kubernetes.io/service-name=kyverno-svc \
      -o jsonpath='{.items[*].endpoints[?(@.conditions.ready==true)].addresses[0]}' 2>/dev/null)" ] && break
    sleep 2
  done
fi

# 2. ArgoCD (it cannot sync itself into existence). Excluded from the local
#    platform ApplicationSet, so this imperative release is authoritative.
echo "→ installing ArgoCD"
helm dependency update infra/helm/platform/argocd >/dev/null
h upgrade --install argocd infra/helm/platform/argocd -n argocd --create-namespace --timeout 8m
k -n argocd rollout status deploy/argocd-server --timeout=300s
k -n argocd rollout status deploy/argocd-repo-server --timeout=300s
k -n argocd rollout status deploy/argocd-applicationset-controller --timeout=300s

# 2b. The edge controller (ADR-0305). kind ships no ingress controller, so the
#     tier installs the committed Traefik chart every
#     environment runs. Imperative, like Cilium: the gateway Application (wave 4)
#     applies IngressRoutes, and their CRDs must exist before it does.
echo "→ installing Traefik (edge controller)"
bash scripts/traefik-install.sh

# 3. SOPS decryption key (the bootstrap root of trust): the committed throwaway
#    local age key, planted as the Secret the sops-operator mounts (ADR-0202).
echo "→ planting sops-age-key (local throwaway key)"
# The namespace comes from the same chart Argo syncs, so it exists with its
# pod-security profile already on it (ADR-0200) rather than bare. `k create
# namespace` here would make it unlabelled, and the label Argo adds a minute later
# would govern only what is admitted after that — not the pods started in between.
h template namespaces infra/helm/platform/namespaces \
  -f infra/gitops/platform/local/values.yaml |
  k apply -f - >/dev/null
k -n "$NS" create secret generic sops-age-key \
  --from-file=keys.txt=infra/gitops/platform/local/age.key \
  --dry-run=client -o yaml | k apply -f -

# 3b. Grafana's `grafana-dashboards` ConfigMap (observability chart values
#     dashboardsConfigMaps.default) is GitOps-managed, not materialised here: the
#     local root-app syncs infra/gitops/local-bootstrap/app-grafana-dashboards.yaml,
#     whose chart generates it from infra/observability/dashboards/*.json at
#     sync-wave 2 — before the core tier (wave 3) starts Grafana, which mounts it
#     (ADR-0500). A PR that adds or edits a dashboard reaches the cluster on the
#     next Argo pass, rather than needing an imperative `kubectl create configmap`.

# 3c. Build + push repo images to the local registry — the local stand-in for CI.
#     Argo then deploys services + lowdefy from the registry exactly as prod pulls
#     from ghcr; the only difference is the registry host in the values overlay
#     (ADR-0205). Must run before step 4 so images exist before Argo creates pods.
#     Build args mirror scripts/service-deploy.sh.
REG="registry.localhost:5000"
# Push over the loopback host, not REG. Docker picks HTTP-vs-HTTPS by resolving the
# registry hostname and checking the result against its insecure-registry CIDRs
# (127.0.0.0/8, ::1/128 by default). The local registry only speaks plain HTTP, so
# the push works only if the name resolves into those CIDRs — which depends on each
# dev's NSS setup mapping *.localhost to loopback (e.g. myhostname). Where it
# doesn't, registry.localhost resolves elsewhere and docker demands TLS: "server
# gave HTTP response to HTTPS client". 127.0.0.1 sidesteps all of that (loopback is
# insecure on every daemon, no per-machine daemon.json). Both names address the
# same registry container and blobs are keyed by repo path, so REG stays the
# cluster-facing pull name in the values overlays; only the push target differs.
PUSH_REG="127.0.0.1:5000"
build_push() { # <image-name> <dockerfile> <context> [extra docker build args…]
  local name="$1" dockerfile="$2" context="$3"
  shift 3
  # Behind a slow proxy, buildkit's fetch of the Dockerfile frontend + base images
  # (docker.io) trips TLS-handshake timeouts intermittently; the layers it did get
  # are cached, so a retry rides over the transient failure. Fails loudly if all
  # attempts miss — same explicit retry the unwedge script uses for host pulls.
  local attempt
  for attempt in 1 2 3; do
    if docker build -t "${PUSH_REG}/${name}:local" -f "$dockerfile" "$@" "$context" &&
      docker push "${PUSH_REG}/${name}:local"; then
      return 0
    fi
    echo "    (build/push of ${name} attempt ${attempt} failed — retrying)"
  done
  echo "✗ could not build+push ${name} after 3 attempts" >&2
  return 1
}
echo "→ building + pushing repo images to ${REG}"
# Build identity baked into each image (ADR-0103): the working-tree SHA (+ -dirty),
# so /version and the X-App-Version header report exactly what this run deployed.
REV="$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"
git diff --quiet 2>/dev/null || REV="${REV}-dirty"
BUILD_ID=(--build-arg "GIT_SHA=${REV}" --build-arg BUILD_VERSION=local
  --build-arg "BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)")
# DERIVED from the values files, not listed here. The services ApplicationSet
# generates one Application per file in that directory, so a file with no image
# built for it is an Application that syncs and then sits in ImagePullBackOff —
# and the tier is "full" in name only. A hand-written list drifts the moment a
# service is added or grows a worker, and it did: `analytics` shipped without a
# server build and `authz` grew a worker without one, so a clean `cluster:full`
# produced two stuck Applications and the fix looked like a missing `cluster:add`
# rather than a missing line here.
#
# `server.enabled` defaults to true and `worker.enabled` to false, matching the
# service chart's own defaults (infra/helm/service/values.yaml).
for values in infra/gitops/services/local/values/*.yaml; do
  svc="$(basename "$values" .yaml)"
  # Apps are not services: they have their own Dockerfile, context and build args,
  # and are built explicitly below.
  [ -f "services/${svc}/Dockerfile" ] || continue
  if [ "$(yq -r '.server.enabled // true' "$values")" = "true" ]; then
    build_push "${svc}-server" "services/${svc}/Dockerfile" . \
      --build-arg SERVICE="${svc}" --build-arg APP_CMD=server "${BUILD_ID[@]}"
  fi
  if [ "$(yq -r '.worker.enabled // false' "$values")" = "true" ]; then
    build_push "${svc}-worker" "services/${svc}/Dockerfile" . \
      --build-arg SERVICE="${svc}" --build-arg APP_CMD=worker "${BUILD_ID[@]}"
  fi
done
build_push admin apps/admin/Dockerfile apps/admin
# The frontend is a deployable service with a values file, so the services
# ApplicationSet generates an Application for it whether or not anything built its
# image — and a full tier without this line sits in ImagePullBackOff until someone
# runs `cluster:add -- frontend` by hand.
#
# It takes one env-specific build arg the Go services do not: `output: "standalone"`
# freezes next.config into server.js, so the server-action CSRF allowlist is decided
# at BUILD time. It is read from the same values file the pod reads it from at
# runtime — one origin, one source (ADR-0306).
build_push frontend apps/frontend/Dockerfile . \
  --build-arg "SERVICE_VERSION=${REV}" \
  --build-arg "EDGE_PUBLIC_ORIGIN=$(yq -r '.env.EDGE_PUBLIC_ORIGIN // ""' infra/gitops/services/local/values/frontend.yaml)"

# 4. Local root App-of-Apps → Argo discovers the local appsets + apps from git.
echo "→ applying local root application"
k apply -f infra/gitops/local-bootstrap/root-application.yaml

# 5. Wait for Argo to converge with `argocd app wait` — it blocks in the
#    foreground, streams per-resource sync/health as it changes, and exits
#    non-zero with the offending resource the moment a sync operation fails
#    (no silent wait to the timeout). Two phases because apps are generated
#    asynchronously: first the root app (its AppProject + 2 child apps [secrets,
#    gateway] + 4 appsets [platform-base/data/core, services]), then
#    every app the appsets produced. Appset generation lags appset sync, so the
#    set is re-listed until stable.
echo "→ waiting for ArgoCD to converge (this is a full platform; first run is slow)…"
AC_KUBECONFIG="$(mktemp)"
trap 'rm -f "$AC_KUBECONFIG"' EXIT
k config view --minify --flatten >"$AC_KUBECONFIG"
kubectl --kubeconfig "$AC_KUBECONFIG" config set-context --current --namespace argocd >/dev/null

# `cluster:stop` freezes cluster state mid-sync; on resume, the ArgoCD controller
# reattaches to whatever sync operation was still "Running" and reuses the task
# plan (incl. per-resource sync-waves) it computed back when that operation
# started — even if the manifests (and their wave annotations) have since
# changed. That stale plan can never converge. If the wait times out, that's the
# first thing to suspect: terminate the wedged operation and force a fresh one
# (which recomputes the plan against current git) before giving up for real.
wait_apps() {
  local timeout="$1"
  shift
  if ac app wait "$@" --sync --health --operation --timeout "$timeout"; then
    return 0
  fi
  echo "⚠ one or more of [$*] did not converge in ${timeout}s — terminating their operations (likely stale from a prior cluster:stop) and forcing a fresh sync"
  local app
  for app in "$@"; do ac app terminate-op "$app" || true; done
  for app in "$@"; do ac app sync "$app" --timeout "$timeout"; done
  ac app wait "$@" --sync --health --operation --timeout "$timeout"
}

wait_apps 600 local-root
while :; do
  apps="$(ac app list -o name)"
  # shellcheck disable=SC2086  # names are newline-separated, intentional split
  wait_apps 1800 $apps
  [ "$(ac app list -o name)" = "$apps" ] && break
done
echo "✓ all ArgoCD applications Synced + Healthy"

# 6. Host-specific edge tail (cannot be GitOps — depends on per-machine state):
#    the /auth + landing routes to a host-run frontend and the frontend-dev
#    EndpointSlice. They live in cluster-edge-glue.sh so the start path
#    (cluster-ensure.sh) and reboot recovery (cluster-heal.sh) can re-stamp them
#    too — otherwise a stop/start drops the `frontend` route (404 at /).
bash scripts/cluster-edge-glue.sh

# 6b. Seed the committed test identities (ADR-0601) so a fresh full tier is usable
#     immediately. Kratos, orgs, and OpenFGA are all up by now, so this creates the
#     identities, runs each one's post-registration process (personal org + tuple),
#     and grants group:operator in OpenFGA — no e2e run or manual ops:grant needed.
#     Idempotent; safe on every bring-up.
bash scripts/identity-seed.sh

cat <<EOF

✓ cluster:full up (ArgoCD-driven from master).
  Product (Traefik):  https://${DOMAIN}:8443/api/<resource>/   (flat namespace, self-signed TLS)
  Ops tier (ADR-0306; coarse gate = operator claim + AAL2, no OpenFGA call):
    Grafana:          https://grafana.ops.${DOMAIN}:8443/
    Hubble UI (map):  https://hubble.ops.${DOMAIN}:8443/
    Temporal UI:      https://temporal.ops.${DOMAIN}:8443/
    SeaweedFS admin:  https://seaweedfs.ops.${DOMAIN}:8443/  (then: admin / seaweedfs-admin-password)
    Lowdefy console:  https://lowdefy.ops.${DOMAIN}:8443/
    ArgoCD:           https://argocd.ops.${DOMAIN}:8443/
    Headlamp (k8s):   https://headlamp.ops.${DOMAIN}:8443/   (read-only debug UI)
    pgweb (DB):       https://pgweb.ops.${DOMAIN}:8443/    (read-only DB inspector)
  Log in:             https://${DOMAIN}:8443/auth/login  — admin@localtest.me / 1st Password!
                      (AAL2: enrol a TOTP second factor on first login; full credential
                      table in docs/dev-loop.md)
  Frontend:           run it natively on :3000 (the frontend-dev EndpointSlice
                      routes /auth + landing to the host).
  Diagnose:           argocd --core --kube-context "$(cluster_ctx)" app get <app>
                      UI: kubectl -n argocd port-forward svc/argocd-server 8080:443
  Break-glass:        auth plane down? reach any tool via kubectl port-forward with
                      your kubeconfig — the sanctioned bypass (docs/guide/break-glass.md),
                      e.g. kubectl -n platform port-forward svc/grafana 3000:80
  Teardown:           mise run cluster:stop  (keep cache) / cluster:delete (delete)
EOF
