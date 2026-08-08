# ADR-0201: GitOps & Deploy

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0101](0101-monorepo.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0200](0200-cluster-topology.md), [ADR-0202](0202-secrets.md), [ADR-0205](0205-environment-parity.md), [ADR-0600](0600-local-development-loop.md)

## Context

Three environments run one cluster each ([ADR-0200](0200-cluster-topology.md)). Deploys cover the service fleet, the frontend, and every platform component including the GitOps controller itself.

This ADR answers how code reaches a cluster, how "what runs in production" stays auditable and revertable, whether services and platform components share deploy machinery, how environments differ without triplicated manifests, and how images are promoted.

## Decision drivers

1. **Pull, not push.** Clusters reconcile themselves against git, and CI never holds cluster credentials.
2. **Same artifact across environments.** An image built once is promoted by SHA, never rebuilt per environment.
3. **Manifest reuse, not duplication.** One chart per concern, environment differences in values.
4. **PRs are the audit log.** Every change to what runs in production is a merged PR, and rollback is `git revert`.
5. **No secret values in git** ([ADR-0202](0202-secrets.md)).

## Considered options

| Option | Fleet fan-out | Ordered bootstrap | Operator UI | Verdict |
| --- | --- | --- | --- | --- |
| **Argo CD** | **ApplicationSet generators** — git-directory, list, cluster — fan out across the fleet without a manifest per service | sync waves, health checks, pre/post-sync hooks | strong; it is the "what is deployed" answer on call | **Chosen.** Apache-2.0, CNCF graduated |
| Flux | Kustomization and HelmRelease per target, or a generator written by hand | dependsOn | none first-party | The fan-out gap is the deciding one: onboarding a service must not require writing a manifest |
| CI pushes with `helm upgrade` | n/a | script order | none | CI holds cluster credentials, and drift is invisible. Fails drivers 1 and 4 |

## Decision

### Argo CD in every cluster

Installed via Helm and managing its own state through the App-of-Apps pattern.

### GitOps state lives in this monorepo

```text
infra/
├── helm/
│   ├── platform/<comp>/        # one chart per platform component
│   └── service/                # one shared chart used by every backend service
└── gitops/
    ├── bootstrap/              # root Application + ApplicationSets
    ├── platform/<env>/values.yaml
    └── services/<env>/values/<svc>.yaml
```

A single repo lets one PR change service code, its chart values, and its deploy state atomically. Splitting into a separate deploy-state repo is a future ADR, triggered when deploy-noise PRs measurably hurt code review.

### Templating

| Target | Chart | Environment differences |
| --- | --- | --- |
| Platform component | `infra/helm/platform/<comp>/` | `infra/gitops/platform/<env>/values.yaml` |
| Backend service | **one shared chart** at `infra/helm/service/` | one values file per service per environment |

The shared chart parameterises image, ports, replicas, environment variables, ingress paths, resource requests and limits, the Temporal worker, the migration init container, and secret references.

Kustomize is not used: Helm values cover environment differences. A case that genuinely cannot be expressed as values gets its own ADR.

Adding a service is the service folder ([ADR-0101](0101-monorepo.md)) plus one values file per environment. The ApplicationSet does the rest.

### Fan-out and ordering

Four ApplicationSets per environment, ordered by sync wave on the root app-of-apps.

| Wave | Set | Components | Gated on |
| --- | --- | --- | --- |
| `-10` | AppProjects | per-env `AppProject`s | — |
| `0` | `platform-base` | sops-operator, cert-manager, network-policies, plus Cilium and Argo CD in prod | — |
| `1` | secrets | the per-env `SopsSecret` | base — the operator and its CRD are up |
| `2` | `platform-data` | postgres, minio | secrets — credentials decrypted |
| `3` | `platform-core` | observability, ory, temporal, openfga, pgweb, headlamp, lowdefy | data — live Postgres, buckets exist |
| `4` | gateway | Traefik middlewares and cross-cutting IngressRoutes | core |
| `5` | services | one Application per service, from a git-directory generator over the values files | gateway |

Cilium and Argo CD sit in the prod base tier and are excluded locally, where they are installed imperatively: a CNI must exist before any pod, and Argo cannot install itself.

**Why four sets rather than sync waves inside one:** sync waves on the Applications a single ApplicationSet generates are **inert**. Those Applications are created by the ApplicationSet controller rather than synced as a parent's resources, so Argo never sequences them among themselves and applies them concurrently. Ordering exists only at the granularity of a root-app-of-apps child, so each tier is its own set.

**What makes the waves real gates:** default ApplicationSet health reflects only successful templating. A custom health check on the `ApplicationSet` kind walks `.status.resources` and reports Progressing until every generated Application is Synced and Healthy, so wave 3 blocks until waves 0 through 2 are up. A companion health check on the CNPG `Cluster` kind makes the data-to-core gate honest — without it Argo reports the unknown CRD Healthy the instant it applies, and core starts against a Postgres that is not ready. Charts within a tier are still concurrent and must tolerate that.

A new service appears in dev the moment its values file lands in `master`, with no Argo configuration change. **This is what keeps onboarding cost flat as the fleet grows.**

### Naming and grouping

Generated Applications are named `<env>-<tier>-<component>`, with env always a prefix: `dev-platform-postgres`, `prod-service-orders`. Env-less cross-cutting apps keep a bare name.

The name is a display convenience. **Grouping uses Argo's real primitives:**

| Primitive | Role |
| --- | --- |
| **AppProject per environment** | Every generated app sets `spec.project: <env>`, making each environment a first-class tenancy boundary with scoped `sourceRepos` and `destinations`. Sync windows live here. The hand-applied `root` seed and the shared `gateway` app stay in the built-in `default` project, because they create the projects or span all environments |
| **Labels** | `env`, `app.kubernetes.io/part-of`, and `app.kubernetes.io/component` on every Application and ApplicationSet, so the UI and `argocd app list -l` filter by concept |

### Image promotion

CI builds one image per service per commit, tagged by SHA ([ADR-0101](0101-monorepo.md)). Promotion is a PR that updates the image reference in an environment's values file.

| Environment | Trigger | Cadence |
| --- | --- | --- |
| dev | merge to `master` opens and auto-merges a values bump; Argo syncs continuously | every merge |
| staging | the same workflow bumps staging simultaneously; a sync window of `05:00 UTC, 1h` batches whatever landed since the last window | daily |
| prod | the release tag `v<YYYY.0M.MICRO>` ([ADR-0103](0103-release-and-versioning.md)) triggers a values bump to the release **digest**, and publishes the release | per release |

**Production pins by digest**, not by tag, because a digest cannot be re-pushed. The promotion workflow gates on rollout completion rather than on Argo reporting Healthy.

The sync-window model batches deploys without ceremony: engineers merge freely, environments cohere on a predictable schedule, and the release act is the one human decision that matters. **Argo CD Image Updater is not used** — it would move deploy state outside the PR that driver 4 makes the audit log.

### Sync policy

| Environment | Platform sync | Services sync | Sync window | `selfHeal` | `prune` |
| --- | --- | --- | --- | --- | --- |
| dev | auto | auto | none | true | true |
| staging | auto | auto | `05:00 UTC, 1h` daily | true | true |
| prod | **manual** | auto, on release tag | none, event-driven | false for platform, true for services | true |

`manualSync: true` on the AppProject permits an ad-hoc `argocd app sync` outside the window for incident response.

**`selfHeal=false` for the production platform** so that a manual intervention during an incident stays visible rather than being silently reverted. Drift raises a notification.

**Retry on failed syncs.** Every Application and ApplicationSet template carries `retry` with `limit: 20` and exponential backoff from 10s to 2m.

`automated` with `selfHeal` does **not** re-run a sync that errored on the same commit, because `selfHeal` reacts only to live drift. Without `retry`, a transient repo-server restart during bootstrap parks the app on a stale `ComparisonError` until someone syncs by hand — most dangerous on the hand-applied root app, whose wave-gated children are never created if it wedges. A manual sync is the break-glass once retries exhaust; a cluster rebuild is never the remedy for a transient failure.

### Bootstrap

After the node provisioning step ([ADR-0200](0200-cluster-topology.md)):

```sh
helm install argocd infra/helm/platform/argocd -n argocd --create-namespace
kubectl apply -f infra/gitops/bootstrap/root-application.yaml
```

Everything else follows from the root Application, and Argo CD is thereafter reconciled by Argo CD. **The first `helm install argocd` is the single non-GitOps action in a cluster's lifetime.**

### Secrets

Encrypted files in the repo are decrypted by an in-cluster operator into native Kubernetes `Secret`s that Helm values reference by name. The mechanism is [ADR-0202](0202-secrets.md).

### Local development

Argo CD is the engine for the full local tier only, from committed `master`, so sync ordering and secret materialisation are exercised as in production. The inner loop runs natively and is not Argo-driven. Both, and the escape hatches for iterating on uncommitted infrastructure, are [ADR-0600](0600-local-development-loop.md).

## Consequences

### Positive

- What runs in production is `git show <sha>`. No ambiguity, no per-environment rebuild.
- Adding a service is a folder plus one values file per environment.
- One shared service chart keeps deploy shape consistent, and a chart change applies to every service at once.
- Image promotion is a reviewable PR, so audit and rollback are git-native.
- Auto-sync where mistakes are cheap, tag-driven where they are expensive, manual where they are bespoke.
- Sync windows carry deploy cadence on the environment rather than on a tag ceremony.

### Negative / Risks

- **The shared service chart is a coupling point.** A breaking change touches every service. Mitigated by chart versioning, CI rendering the chart against every service's values, and a chart-change review checklist.
- **Single-repo deploy state mixes image-bump PRs with feature PRs.** Mitigated by a title prefix and a separate code owner on the GitOps tree. Splitting repos is a future ADR.
- **Staging batches by window**, so merge-by-merge behaviour is not observable there. Accepted — it is the explicit goal, and a manual sync is available when the next window is too far away.
- **A bad merge sits on `master` until the next staging window.** Mitigated by full CI on PRs and by dev continuously running `master`.
- **Argo CD itself can fail.** Mitigated by an HA install in production. Downtime blocks new syncs; running workloads are unaffected.
- **Sync waves gate on custom health checks we maintain.** A bug there produces a false ordering guarantee, which is worse than none. Mitigated by the checks being small, committed, and exercised on every local full-tier bring-up.

### Follow-ups

- `infra/helm/platform/argocd/` values — HA in production, single replica elsewhere.
- `infra/helm/service/` shared chart.
- `infra/gitops/bootstrap/root-application.yaml` and the four ApplicationSets.
- `tools/promote/` opening the values-bump PR for dev and staging on merge, and for production on a release tag.
- `helm template` snapshot tests, `helm lint`, and `kubeconform` in CI.
- `docs/gitops/runbook.md` covering sync failures, drift, rollback, and fresh-cluster bootstrap.

## Rules

- Argo CD is the only mechanism that applies manifests to a cluster. `kubectl apply` is permitted only for the one-time bootstrap step. `(review-only)`
- Every backend service is deployed through the shared chart with per-env values files; platform components have one chart each. `(CI: lint:service-contract)`
- Environment differences live in values files, never in chart logic conditioned on the environment name. `(review-only)`
- An image is built once and promoted by updating values files. Rebuilding for another environment is not done. `(CI: ci-build)`
- Promotion to dev and staging is automatic on merge, with cadence enforced by sync windows. Promotion to production is automatic on a release tag and pins by digest. `(review-only)`
- No environment is deployed by hand-opening a values-bump PR. `(review-only)`
- Argo CD Image Updater and similar auto-promoters are not used. `(review-only)`
- Production platform syncs are manual with `selfHeal=false`; production services sync automatically with `selfHeal=true`. `(review-only)`
- Every Application and ApplicationSet carries a `retry` policy. `(review-only)`
- Secret values never appear in git; manifests carry SOPS-encrypted files or references to the Secrets they produce. `(CI: lint:secrets)`
