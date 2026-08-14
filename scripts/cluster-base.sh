#!/usr/bin/env bash
# `cluster:base` (ADR-0205, ADR-0600) — the local floor every service needs.
#
#   Traefik · cert-manager · Postgres · Kratos · Oathkeeper   real
#   Temporal · OpenFGA · CNPG · observability · ArgoCD        absent, opt-in per service
#
# Everything above the floor is opt-in, declared by the service that needs it
# (`dep:*` in each `services/<svc>/.mise.toml`) rather than by a named profile.
# There is no `min`/`backend`/`edge`/`obs` tier: what is up is base ∪ the declared
# dependencies of whatever you are running. See ADR-0205.
#
# The identity stack is in the floor deliberately (ADR-0600's argument, generalised
# from the frontend to every service): Kratos and Oathkeeper are cheap, and their
# behaviour — cookies, CSRF, session expiry, the 401/403 — IS the contract every
# service consumes. A service reached through this edge gets real Oathkeeper
# identity headers; one curled directly is being handed forged ones. So the auth
# path is byte-identical to production — no bypass, no development provider, no
# NODE_ENV branch, because nothing needs one.
#
# This is a SELECTION, not a copy: every component is installed from the same chart
# and the same `infra/gitops/platform/local/values.yaml` overlay that cluster:full
# uses. This script names which ones, and nothing else — no second values file to
# drift.
#
# Not ArgoCD-driven, deliberately: Argo is the full tier's engine (ADR-0205), and
# this is the inner loop.
#
# Related: `scripts/dep-apply.sh` adds one opt-in component on top of this floor;
# `mise run cluster:edge-glue` re-stamps the host edge glue alone (hidden; this
# script and the start/reboot paths already run it for you).
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

CLUSTER="${CLUSTER:-platform}"
NS="platform"
DOMAIN="${DOMAIN:-dev.localtest.me}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }
h() { helm --kube-context "k3d-${CLUSTER}" "$@"; }

# 1. Cluster + CNI. Same bootstrap floor as every other tier.
bash scripts/cluster-ensure.sh
bash scripts/cilium-install.sh

# 2. Postgres only, out of the shared dependency manifest. Temporal and OpenFGA
#    live in the same file and are skipped by label — they are opt-in, added by the
#    services that declare them (`dep:temporal`, `dep:openfga`). Postgres is in the
#    floor because Kratos needs a store, so every base has one anyway.
step "applying the postgres dependency stand-in (Kratos's store)"
k apply -f infra/local/deps.yaml -l 'local.platform/component in (base,postgres)'
k -n "$NS" rollout status deploy/postgres --timeout=180s

# 3. TLS. cert-manager IS the mechanism in every environment (ADR-0205); only the
#    issuer differs, and the local values already select the SelfSigned one.
#    Helm applies the chart's ClusterIssuer/Certificates in the same pass that
#    creates the webhook they must be admitted by, so a cold install can lose that
#    race ("no endpoints available for service cert-manager-webhook"). ArgoCD
#    solves it with sync-waves; imperatively the honest fix is to wait for the
#    webhook and apply again — the retry is doing what wave 1 does, not papering
#    over a flake.
step "installing cert-manager + the self-signed wildcard issuer"
# `build` reads Chart.lock and needs the dependency's repository already
# registered in the caller's helm config; `update` resolves it from the URL in
# Chart.yaml. On a developer machine the repo is usually registered from some
# earlier run, so `build` alone works and keeps working — and fails on a clean
# runner with "no repository definition", which is exactly what happened the
# first time CI ran this. Build first because it is the cheap offline path;
# fall back to update, which is what every other script here already uses.
h dependency build infra/helm/platform/cert-manager >/dev/null 2>&1 ||
  h dependency update infra/helm/platform/cert-manager >/dev/null
for attempt in 1 2 3; do
  if h upgrade --install cert-manager infra/helm/platform/cert-manager \
    -n "$NS" --create-namespace --timeout 5m --wait \
    -f infra/gitops/platform/local/values.yaml; then
    break
  fi
  [ "$attempt" = 3 ] && fail "cert-manager did not install after 3 attempts"
  detail "waiting for the cert-manager webhook, then retrying"
  k -n "$NS" rollout status deploy/cert-manager-webhook --timeout=180s || true
done
k -n "$NS" wait --for=condition=Ready certificate/wildcard --timeout=120s

# 4. The shared edge middlewares (ADR-0305): identity-header stripping, Oathkeeper
#    forward-auth, the rate limit, security headers. Only middlewares.yaml — the
#    rest of infra/gateway routes the ops tier, which this profile does not run.
step "applying the edge middlewares"
k apply -n "$NS" -f infra/gateway/middlewares.yaml

# 5. Kratos's secrets. The committed local SOPS bundle is the source (same material
#    cluster:full decrypts through the sops-operator), with one substitution: the
#    dsn points at the stand-in Postgres above instead of CNPG, which this profile
#    does not run.
step "materialising kratos-secrets from the committed local SOPS bundle"
secrets="$(SOPS_AGE_KEY_FILE=infra/gitops/platform/local/age.key \
  sops -d infra/gitops/platform/local/secrets/platform.enc.yaml |
  yq -o=json '.spec.secretTemplates[] | select(.name == "kratos-secrets") | .stringData')"
[ -n "$secrets" ] || fail "kratos-secrets not found in the local SOPS bundle"
kratos_dsn="postgres://dev:dev@postgres.${NS}.svc.cluster.local:5432/kratos?sslmode=disable"
k -n "$NS" create secret generic kratos-secrets \
  --from-literal=secretsDefault="$(jq -r .secretsDefault <<<"$secrets")" \
  --from-literal=secretsCookie="$(jq -r .secretsCookie <<<"$secrets")" \
  --from-literal=secretsCipher="$(jq -r .secretsCipher <<<"$secrets")" \
  --from-literal=smtpConnectionURI="$(jq -r .smtpConnectionURI <<<"$secrets")" \
  --from-literal=dsn="$kratos_dsn" \
  --dry-run=client -o yaml | k apply -f -

# 6. Kratos + Oathkeeper, wired exactly the way the platform ApplicationSet wires
#    them: the canonical infra/auth/ overlays plus the string artefacts, never
#    inlined into the chart values (lint:auth-inline).
step "installing kratos + oathkeeper"
# Same fallback as cert-manager above.
h dependency build infra/helm/platform/ory >/dev/null 2>&1 ||
  h dependency update infra/helm/platform/ory >/dev/null
h upgrade --install ory infra/helm/platform/ory \
  -n "$NS" --create-namespace --timeout 8m --wait \
  -f infra/auth/kratos/values.yaml \
  -f infra/auth/oathkeeper/values.yaml \
  -f infra/gitops/platform/local/values.yaml \
  --set-file 'kratos.kratos.identitySchemas.user\.v1\.json=infra/auth/kratos/identity-schemas/user.v1.json' \
  --set-file 'oathkeeper.oathkeeper.accessRules=infra/auth/oathkeeper/access-rules.json'

# 7. Make the env host resolve to Traefik from inside the cluster, so a pod that
#    dials the edge reaches the edge and not its own loopback. Needed by anything
#    running in-cluster that calls through the edge — the frontend above all.
step "rewriting ${DOMAIN} to the edge in CoreDNS"
k apply -f infra/local/coredns-rewrite.yaml
k -n kube-system rollout restart deploy/coredns
k -n kube-system rollout status deploy/coredns --timeout=120s

# 8. Host edge glue: the catch-all `/` route to the host `next dev` and the
#    docker-bridge EndpointSlice. cluster-ensure.sh skips it on a brand-new cluster
#    (Traefik's CRDs are not registered yet), so stamp it now that they are.
k apply -f infra/local/traefik-config.yaml
bash scripts/cluster-edge-glue.sh

# 9. The committed test identities (ADR-0601) — the same ones the e2e suite uses,
#    not a parallel development-only identity.
bash scripts/identity-seed.sh

# The credentials are committed once, in the e2e fixtures; read them rather than
# repeating them here (bun reads the .ts directly — no Node, no e2e install).
login_email="$(bun --silent -e \
  "console.log((await import('${ROOT}/test/e2e/fixtures/identities.ts')).USER.email)" 2>/dev/null ||
  echo '<see test/e2e/fixtures/identities.ts>')"

cat <<EOF

✓ cluster:base up — real edge + real identity + Postgres.

  Add what you need on top — nothing else is running:
    cd services/<svc> && mise run server    native, pulls its own deps in
    mise run cluster:add -- <svc>           in-cluster, from the working tree

  Open:          https://${DOMAIN}:8443/
  Log in as:     ${login_email}
                 (password: test/e2e/fixtures/identities.ts — sessions last 7 days)
  Teardown:      mise run cluster:stop  (keep cache) / cluster:delete (delete)

  Base alone serves no application data: /api routes 404 until something answers
  behind them — a service you add, or the contract mock:
    mise run mock:start                     /api from the committed OpenAPI

  The auth path is complete either way — logging in, CSRF, expiry and the
  Oathkeeper 401/403 all behave as they do in production.

  Persistence here is ephemeral (the Postgres stand-in has no volume) and no
  workflow runs without dep:temporal. Assert persistence, Temporal behaviour and
  authorization decisions against real services on cluster:full.
EOF
