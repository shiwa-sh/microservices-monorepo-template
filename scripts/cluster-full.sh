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

# The full tier runs on Talos, not kind (ADR-0600). Set BEFORE the context library
# is sourced: it resolves the cluster name and kube context from this, and a
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
# The Talos provisioner installs Cilium itself, and it has to: this tier's machine
# config carries `cni: name: none`, so the nodes never go Ready — and
# `talosctl cluster create` never returns — until a CNI is delivered. There is no
# separate cilium-install step here for that reason.
bash scripts/cluster-dispatch.sh ensure
# On a proxied network the node's containerd can wedge pulling the large Cilium /
# ArgoCD images through privoxy, stalling the `helm --wait` steps below (which run
# before Argo, so cluster:unwedge can't rescue them). CLUSTER_PRELOAD=1 warms those
# images on the host first. Off by default — clean networks never need it.
if [ "${CLUSTER_PRELOAD:-0}" = "1" ]; then
  echo "→ CLUSTER_PRELOAD=1: preloading bootstrap-critical images"
  bash scripts/cluster-preload-images.sh
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
for svc in authz catalog orders orgs payment; do
  build_push "${svc}-server" "services/${svc}/Dockerfile" . \
    --build-arg SERVICE="${svc}" --build-arg APP_CMD=server "${BUILD_ID[@]}"
done
for svc in orders orgs payment; do
  build_push "${svc}-worker" "services/${svc}/Dockerfile" . \
    --build-arg SERVICE="${svc}" --build-arg APP_CMD=worker "${BUILD_ID[@]}"
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

# 3d. Hand the inner loop's stand-ins back to GitOps.
#
#     `cluster:base` (ADR-0600) runs a throwaway Postgres Deployment named
#     `postgres` and writes `kratos-secrets` imperatively, with a dsn pointing at
#     it. This tier brings CNPG (`postgres-rw`), network policies that permit only
#     that path, and the same Secret from the SOPS bundle carrying the CNPG dsn.
#     Left alone, the transition ends badly and silently:
#
#       - the sops-operator refuses to adopt a Secret it did not create
#         ("Child secret is not owned by controller"), so the dsn stays the base one
#       - Kratos keeps dialling `postgres`, which the policies now deny
#       - every Kratos admin call hangs to its timeout, which surfaces as `mise run
#         e2e` failing in the identity bootstrap with a bare 60s timeout
#
#     So the stand-in goes, and the imperative Secret with it. Both are deleted BY
#     NAME. The label `cluster:base` selects on (`local.platform/component=base`)
#     is also on the Namespace, so handing that selector to `delete` takes the whole
#     platform down — the one thing this step must not do.
#
#     What is deliberately left alone: the `ory` and `cert-manager` Helm releases
#     the base tier installs. Argo adopts their resources through ServerSideApply
#     and reconciles them to the same charts, so the release records are inert
#     bookkeeping. Uninstalling them would delete workloads Argo is about to
#     recreate, for no gain.
#     All THREE stand-ins, not just Postgres. `dep:temporal` and `dep:openfga` are
#     opt-in on the inner loop, so whether they exist depends on what someone ran
#     there — and the one that bites is the quiet one: the stand-in Deployment and
#     the chart's carry different `spec.selector` values, which is IMMUTABLE, so
#     ArgoCD retries the update twenty times and gives up with "field is immutable".
#     The app then sits OutOfSync forever, the root app never leaves its wave, and
#     nothing says why. Measured on openfga, after Postgres alone had been handled.
for stand_in in postgres temporal openfga; do
  if k -n "$NS" get deploy "$stand_in" >/dev/null 2>&1; then
    echo "→ removing the inner loop's ${stand_in} stand-in (the platform chart owns it here)"
    k -n "$NS" delete "deploy/${stand_in}" "svc/${stand_in}" --ignore-not-found >/dev/null
  fi
done
k -n "$NS" delete cm/postgres-initdb --ignore-not-found >/dev/null
if k -n "$NS" get secret kratos-secrets >/dev/null 2>&1 &&
  [ -z "$(k -n "$NS" get secret kratos-secrets -o jsonpath='{.metadata.ownerReferences}')" ]; then
  echo "→ handing kratos-secrets back to the sops-operator"
  k -n "$NS" delete secret kratos-secrets >/dev/null
  # The operator reconciles on SopsSecret changes, not on the disappearance of a
  # child Secret, so a restart is what makes it look again. No-op when it is not
  # installed yet, which is the case on a cluster that never ran the base tier.
  k -n "$NS" rollout restart deploy/sops-operator-sops-secrets-operator >/dev/null 2>&1 || true

  # Kratos reads the dsn from the Secret through the environment, which is fixed at
  # pod start — so the running pod is still on the base dsn until it is replaced.
  # Wait (briefly) for the operator to put the Secret back, so the new pod starts
  # with it rather than into a CreateContainerConfigError it would retry out of.
  if k -n "$NS" get deploy ory-kratos >/dev/null 2>&1; then
    for _ in $(seq 1 30); do
      k -n "$NS" get secret kratos-secrets >/dev/null 2>&1 && break
      sleep 2
    done
    k -n "$NS" rollout restart deploy/ory-kratos >/dev/null
  fi
fi

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
