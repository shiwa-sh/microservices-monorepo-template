# ADR-0004: GitOps & Deploy

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0002](0002-monorepo.md), [ADR-0003](0003-cluster-topology.md), [ADR-0005](0005-secrets.md), [ADR-0013](0013-release-and-versioning.md)

## Context

Three environments — **dev**, **staging**, **prod** — each on a k3s cluster ([ADR-0003](0003-cluster-topology.md)). Deploys cover ~100 backend services, the frontend, and platform components (Postgres via CNPG, Temporal, Kratos/Hydra, SpiceDB, Tyk, the observability stack, ArgoCD itself).

We need a single answer to:

- How code reaches a cluster.
- How "what runs in prod" stays auditable, reproducible, and revertable.
- How application services and platform components share — or do not share — deploy machinery.
- How environments differ without duplicating manifests three times.
- How images are promoted across environments.

## Decision drivers

1. **Pull, not push.** Clusters reconcile themselves against git. CI never holds cluster credentials.
2. **Same artifact across environments.** A container image built once is promoted by SHA; never rebuilt per environment.
3. **Manifest reuse, not duplication.** One Helm chart per concern, env differences in values files.
4. **PRs are the audit log.** Every change to what-runs-in-prod is a merged PR; rollback is `git revert`.
5. **No secret values in git.** References yes, values no — see [ADR-0005](0005-secrets.md).

## Decisions

### Engine: ArgoCD

ArgoCD is the GitOps controller in every cluster, installed via Helm and managing its own state via the App-of-Apps pattern.

**Why ArgoCD over Flux:**

- **ApplicationSet** generators (git-directory, list, cluster) handle fan-out across 100 services without writing 100 manifests. This is the most important feature at our scale.
- **Strong UI.** On-call engineers and developers use it as the "what is actually deployed?" answer.
- **Sync waves, health checks, pre/post-sync hooks.** Ordered platform bootstrap (CRDs → operators → instances) works without external orchestration.
- Apache 2.0, CNCF graduated.

### Repository topology: single repo

GitOps state lives **in this monorepo**, alongside application code.

```text
infra/
├── helm/
│   ├── platform/<comp>/        # one chart per platform component
│   └── service/                # one shared chart used by every backend service
└── gitops/
    ├── bootstrap/              # root Application + ApplicationSets
    ├── platform/
    │   ├── dev/values.yaml     # per-env platform overrides
    │   ├── staging/values.yaml
    │   └── prod/values.yaml
    └── services/
        ├── dev/values/<svc>.yaml
        ├── staging/values/<svc>.yaml
        └── prod/values/<svc>.yaml
```

A single repo means a PR can change service code, its chart values, and its deploy state atomically. The split into a separate `deploy-state` repo is a future ADR, triggered only when deploy-noise PRs hurt code review.

### Templating: Helm with per-environment values

- **Platform components** — chart at `infra/helm/platform/<comp>/`; env values at `infra/gitops/platform/<env>/values.yaml`.
- **Backend services** — **one shared chart** at `infra/helm/service/` used by every service. Per-service difference is one values file per environment. The chart parameterises: image, ports, replicas, env vars, ingress paths, resource requests/limits, the Temporal worker sidecar, the migration init container, and ExternalSecret references.
- **Kustomize is not used.** Helm values cover env differences. If a future case genuinely cannot be expressed as values, it gets its own ADR.

Adding a new service is: create the service folder ([ADR-0002](0002-monorepo.md)), add one values file per env. Three small files; ApplicationSet does the rest.

### Fan-out: ApplicationSet per environment

Two ApplicationSets per environment:

1. **Platform ApplicationSet** — list generator over `infra/helm/platform/*`. Sync waves order CRDs → operators → instances.
2. **Services ApplicationSet** — git-directory generator over `infra/gitops/services/<env>/values/*.yaml`. One Application per service per environment, created and deleted automatically as files are added and removed.

A new service appears in dev the moment its `dev/values/<svc>.yaml` lands in `master`. No ArgoCD config changes required to onboard a service. This is the property that makes 100 services tractable for an 8-engineer team.

### Image promotion

CI builds one image per service per commit, tagged `<service>:<git-sha>` ([ADR-0002](0002-monorepo.md)). Promotion is
by **PR that updates the `image.tag` field in the env's values file**. Dev and staging promotions are automatic on
`master` merge; cadence is controlled by ArgoCD **sync windows**. Prod promotion is **tag-driven** per
[ADR-0013](0013-release-and-versioning.md).

- **dev:** merging to `master` triggers `.github/workflows/promote-on-merge.yml`, which opens (and auto-merges) a PR
  bumping `infra/gitops/services/dev/values/<svc>.yaml` to the new SHA. ArgoCD dev syncs continuously. Expected
  cadence: every merge (multiple times per day).
- **staging:** the same workflow simultaneously bumps `infra/gitops/services/staging/values/<svc>.yaml`. ArgoCD staging
  has a sync window of `05:00 UTC, 1h` — staging picks up whatever values landed since the previous window in a single
  batch. `manualSync: true` keeps an out-of-band `argocd app sync` available for incident response. Expected cadence:
  once daily.
- **prod:** cutting a release tag `services/<svc>/v<X.Y.Z>` ([ADR-0013](0013-release-and-versioning.md)) triggers
  `.github/workflows/promote-on-release.yml`, which labels the image with the semver tag, opens (and auto-merges) a PR
  bumping `infra/gitops/services/prod/values/<svc>.yaml` to the release SHA, and publishes a GitHub Release. ArgoCD
  prod auto-syncs services. Expected cadence: per release.

The sync-window model batches deploys without ceremony: engineers merge freely, environments cohere on a predictable
schedule, and the release act is the one human decision that matters. **ArgoCD Image Updater is not used.**

### Sync policy

| Environment | Platform sync | Services sync         | Sync window           | `selfHeal`                         | `prune` |
|-------------|---------------|-----------------------|-----------------------|------------------------------------|---------|
| dev         | auto          | auto                  | none (continuous)     | true                               | true    |
| staging     | auto          | auto                  | `05:00 UTC, 1h` daily | true                               | true    |
| prod        | manual        | auto (on release tag) | none (event-driven)   | false (platform) / true (services) | true    |

The staging sync window is configured on the AppProject; `manualSync: true` permits ad-hoc `argocd app sync` outside
the window for incident response.

**Why `selfHeal=false` for prod platform:** manual interventions during an incident must be visible, not silently
reverted. Drift alerts fire to Slack via ArgoCD notifications.

### Bootstrap

A new cluster bootstraps with two commands after [ADR-0003](0003-cluster-topology.md)'s Terraform + Ansible steps:

```sh
helm install argocd infra/helm/platform/argocd -n argocd --create-namespace
kubectl apply -f infra/gitops/bootstrap/root-application.yaml
```

The root Application points at `infra/gitops/bootstrap/`, which contains the platform and services ApplicationSets. Everything else follows. ArgoCD itself is then reconciled by ArgoCD; the single non-GitOps action in a cluster's lifetime is the first `helm install argocd`.

### Secrets

Secret values are not in git. Encrypted SOPS files in the repo are decrypted by an in-cluster operator and exposed as Kubernetes `Secret`s referenced by Helm values. The full mechanism is [ADR-0005](0005-secrets.md).

### Local development

GitOps is **not used locally.** `mise run dev:up` runs `helm install` directly against k3d:

- Engineers iterate on Helm chart changes without committing.
- ArgoCD itself is a component under test in some workflows; running it locally adds startup time without value.

A `mise run dev:gitops` task installs ArgoCD locally for engineers debugging the GitOps layer specifically.

## Consequences

### Positive

- What runs in prod is `git show <sha>`. No ambiguity, no per-environment rebuild.
- Adding a service is a folder plus one values file per env. ApplicationSet handles the rest.
- One shared service chart keeps deploy shape consistent; chart changes apply to every service at once.
- Image promotion is a reviewable PR. Audit and rollback are git-native.
- Auto-sync where mistakes are cheap (dev/staging); tag-driven where they are expensive (prod services); manual where
  they are bespoke (prod platform).
- Sync windows batch deploys without introducing a release-candidate tag ceremony — the environment, not the tag,
  carries the cadence.

### Negative / Risks

- The shared service chart is a coupling point. A breaking change to it touches every service. Mitigated by chart versioning, chart-only CI rendering against every service's values, and a chart-change PR review checklist.
- Single-repo deploy state means image-bump PRs mix with feature PRs. Mitigated by a `[deploy]` PR title prefix and a separate code-owner on `infra/gitops/services/`. Splitting repos is a future ADR.
- Staging batches by sync window; engineers cannot observe merge-by-merge behaviour in staging. Accepted — this is the
  explicit goal (see [ADR-0013](0013-release-and-versioning.md)). Manual `argocd app sync` is available when the next
  window is too far away.
- A bad merge sits on `master` until the next staging window surfaces it. Mitigated by full CI on PRs and by dev
  continuously running master.
- ArgoCD itself can fail. Mitigated by HA install in prod (3 replicas, redis HA). ArgoCD downtime blocks new syncs; running workloads are unaffected.

### Follow-ups

- `infra/helm/platform/argocd/` chart values (HA in prod, single-replica in dev/staging).
- `infra/helm/service/` shared backend service chart.
- `infra/gitops/bootstrap/root-application.yaml` and both ApplicationSets.
- `tools/promote/` Go program: open the values-bump PR for dev + staging (on merge) and for prod (on release tag).
- `.github/workflows/promote-on-merge.yml` for dev + staging value bumps on `master` merge.
- `.github/workflows/promote-on-release.yml` for prod value bump + GitHub Release on tag push.
- AppProject manifests with the `05:00 UTC` staging sync window and `manualSync: true`.
- `helm template` snapshot tests in CI; `helm lint` and `kubeconform` on the chart.
- `docs/gitops/runbook.md` covering sync failures, drift, rollback, and fresh-cluster bootstrap.

## Rules

- ArgoCD is the only mechanism that applies manifests to a cluster. `kubectl apply` is permitted only for the one-time bootstrap step.
- Every backend service is deployed via the shared chart at `infra/helm/service/` with per-env values files.
- Platform components have one chart per component under `infra/helm/platform/`.
- Env differences live in values files, never in chart logic conditioned on `.Values.environment`.
- A container image is built once with tag `<service>:<git-sha>` ([ADR-0002](0002-monorepo.md)) and promoted by updating values files. Rebuilding for another environment is forbidden.
- Promotion to dev and staging is automatic on `master` merge. Deploy cadence is enforced by ArgoCD sync windows — dev continuous, staging `05:00 UTC` daily.
- Promotion to prod is automatic on a release tag (`services/<svc>/v<X.Y.Z>`) per [ADR-0013](0013-release-and-versioning.md). No environment is deployed by hand-opening a values-bump PR.
- ArgoCD Image Updater and similar auto-promoters are not used.
- Prod platform syncs are manual with `selfHeal=false`. Prod services sync automatically with `selfHeal=true`.
- Secret values never appear in git. Manifests carry SOPS-encrypted files or ExternalSecret references; see [ADR-0005](0005-secrets.md).
- Local development uses `helm install` directly against k3d; ArgoCD is not part of the local default loop.
