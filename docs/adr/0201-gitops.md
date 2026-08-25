# ADR-0201: GitOps & Deploy

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0101](0101-monorepo.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0200](0200-cluster-topology.md), [ADR-0202](0202-secrets.md), [ADR-0205](0205-environment-parity.md), [ADR-0600](0600-local-development-loop.md)
- **Decides:** Argo CD is the only deploy mechanism, reconciling one shared Helm chart from this repository with per-environment values.

## Context

Three environments run one cluster each ([ADR-0200](0200-cluster-topology.md)). Deploys cover the service fleet, the frontend, and every platform component including the GitOps controller itself.

This ADR answers how code reaches a cluster, how "what runs in production" stays auditable and revertable, whether services and platform components share deploy machinery, how environments differ without triplicated manifests, and how images are promoted.

## Decision drivers

1. **The cluster reconciles itself.** Nothing outside the cluster holds credentials to it, and divergence from the declared state is reported rather than assumed away.
2. **Onboarding cost stays flat as the fleet grows.** Adding the Nth service must not add an Nth hand-written deploy manifest.
3. **Bootstrap order is expressible.** A CNI before any pod, a secret operator before the secrets, a database before its consumers.
4. **PRs are the audit log.** Every change to what runs in production is a merged PR, and rollback is `git revert`.
5. **One chart per concern, differences in values.** Environment variation must not become a second copy of the manifest.

Two constraints are inherited rather than decided here, and select nothing below because every option satisfies them: an image is built once and promoted by SHA ([ADR-0101](0101-monorepo.md)), and no secret value reaches git in plaintext ([ADR-0202](0202-secrets.md)).

## Considered options

### The controller

| Option | Fleet fan-out | Ordered bootstrap | Operator UI | Verdict |
| --- | --- | --- | --- | --- |
| **Argo CD** | **[ApplicationSet generators](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators/)** — git-directory, list, cluster — fan out across the fleet without a manifest per service | sync waves, health checks, pre/post-sync hooks | strong; it is the "what is deployed" answer on call | **Chosen.** Apache-2.0, CNCF graduated *(documented)* |
| Flux | Kustomization and HelmRelease per target, or a generator written by hand | `dependsOn` | none first-party | Driver 2 decides it: onboarding a service must not require writing a manifest |
| Flux with a third-party UI | as above | as above | Flamingo renders Flux resources through Argo's UI | Adds a second project to the floor to recover what the other option ships, and leaves driver 2 unanswered |
| CI pushes with `helm upgrade` | n/a | script order | none | CI holds cluster credentials and drift is invisible. Fails drivers 1 and 4 |

### Templating

| Option | Environment differences | Cost of the Nth service | Verdict |
| --- | --- | --- | --- |
| **Helm values** | one values file per target | one values file against a shared chart | **Chosen.** Driver 5 in its cheapest form, and the ecosystem ships charts, so third-party components need no translation *(reasoned)* |
| Kustomize overlays | a base plus a patch directory per environment | a base plus one overlay directory per environment | Patches match the base structurally, so a base change can silently no-op a patch. Directories multiply by services × environments, against driver 2 |
| Helm rendered, then Kustomize post-render | both | both | Two templating systems for one concern, which principle 5 refuses |
| CUE, Timoni, or jsonnet | typed and composable | strong | A third language in the deploy layer, and every upstream component still arrives as a chart to be wrapped |
| A manifest per environment — the honest baseline | copies | three copies | Driver 5 exists to refuse exactly this |

### Ordering generated Applications

| Option | Ordering it can express | Effect on sync policy | Verdict |
| --- | --- | --- | --- |
| **Four ApplicationSets under a root app-of-apps** | between tiers, by sync wave on the root's children | none — each generated Application keeps its own automated sync | **Chosen.** Ordering lands at the only granularity Argo sequences, and the sync policy stays per-environment *(reasoned)* |
| One ApplicationSet, sync waves on the generated Applications | **none** — the waves are inert, see below | none | The intuitive reading of the feature, and it does not work |
| One ApplicationSet with `RollingSync` progressive syncs | between steps within the set | **forces automated sync off** on every generated Application | Purpose-built for ordering, and it buys that ordering by removing the continuous reconciliation driver 1 is here for |

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

A single repo lets one PR change service code, its chart values, and its deploy state atomically.

**Splitting out a deploy-state repo is deferred.**

| Field | Value |
| --- | --- |
| **Trigger** | values-bump PRs outnumber code PRs on `master`, or a code review is opened against a diff that is more than half values bumps |
| **Seam** | ✓ deploy state is already one subtree, `infra/gitops/`, and every Application's `source` names a repo URL and a path. Moving it changes those URLs and the CI job that opens bump PRs |
| **Cost if adopted late** | history for the moved subtree either splits or is rewritten, and every open values-bump PR is invalidated at the cut. The cost grows with the number of environments, not with time |

### Templating

| Target | Chart | Environment differences |
| --- | --- | --- |
| Platform component | `infra/helm/platform/<comp>/` | `infra/gitops/platform/<env>/values.yaml` |
| Backend service | **one shared chart** at `infra/helm/service/` | one values file per service per environment |

The shared chart parameterises image, ports, replicas, environment variables, ingress paths, resource requests and limits, the Temporal worker, the migration init container, and secret references.

Kustomize is not used. A difference that cannot be expressed as a value is a chart change, not an overlay.

Adding a service is the service folder ([ADR-0101](0101-monorepo.md)) plus one values file per environment. The ApplicationSet does the rest.

### Fan-out and ordering

Five ApplicationSets per environment, ordered by sync wave on the root app-of-apps. **Each wave is named for the reason it must precede the next**, and a chart belongs to the last wave whose successors still resolve it — so the ladder is a claim a reader can check, chart by chart, rather than a layering to memorise.

| Wave | Set | Components | Why it precedes the next |
| --- | --- | --- | --- |
| `-10` | AppProjects | per-env `AppProject`s | an Application cannot cite a project that does not exist |
| `0` | `platform-admission` | namespaces + PSA, kyverno, priority classes, resource-governance, network-policies, cert-manager, sops-operator, local-path | admission is evaluated once, at creation: a policy arriving later has already missed what it gates |
| `1` | secrets | the per-env `SopsSecret` | the operator that decrypts it is up |
| `2` | `platform-stores` | postgres, seaweedfs | every wave below reads one of them, and they need the decrypted credentials |
| `3` | `platform-telemetry` | observability, alertmanager | `otel-agent` exports to `tempo` and `loki` **by name**, so the backends cannot follow the collector |
| `4` | `platform-core` | zot, otel-agent, ory, edge-errors, mailpit, maddy, temporal, openfga, public-tls | everything a service resolves by name |
| `5` | gateway | Traefik middlewares and cross-cutting IngressRoutes | its `problem-json-errors` middleware names `edge-errors` |
| `6` | services | one Application per service, from a git-directory generator over the values files | — |
| `7` | `platform-consoles` | pgweb, headlamp, lowdefy | nothing resolves a console |

**Two placements are worth stating, because the rule alone does not predict them.** `zot` sits in core although no DNS name resolves it: deployed nodes pull images from it, so it precedes anything that starts later, and it needs the wave-2 bucket. The mounted-config Applications — Prometheus rules, Grafana dashboards, Alertmanager silences — sit one wave *before* the tier that mounts them, because a pod whose ConfigMap volume is missing cannot start at all.

**A wave holds only what a later wave resolves.** An operator console is a read surface over the platform ([ADR-0501](0501-operator-uis-and-dashboards.md)): no workload looks one up, and the gateway's ops routes are `IngressRoute`s that answer as soon as their backend appears. A wave that holds a console makes every later wave wait for a dashboard, and puts its pods on the node while the store's consumers are still electing.

The rule cuts the other way too, and that is the half easier to get wrong: a component nothing resolves may still be late, but a component something resolves may never be. Telemetry is the case in point — the collector names its backends, so the backends precede it, and no reordering for speed may cross that edge.

Cilium and Argo CD are in no tier, in any environment. Both are installed imperatively before Argo runs, which is the one-time bootstrap step this ADR permits: no pod schedules before the CNI exists, and Argo cannot apply its own first install.

Managing either through this set also breaks it. Every generated Application targets the `platform` namespace, while Cilium runs in `kube-system` and Argo CD in `argocd`, and both charts carry cluster-scoped RBAC — the rendered `ClusterRoleBinding` keeps its name and rebinds the subject to `platform:<sa>`, overwriting the correct binding. The component then loses the access its own `ClusterRole` grants while continuing to report healthy.

**Why the waves inside one set are inert:** Applications a single ApplicationSet generates are created by the ApplicationSet controller rather than synced as a parent's resources, so Argo never sequences them among themselves and applies them concurrently. Ordering exists only at the granularity of a root-app-of-apps child, which is why each tier is its own set.

**What makes the gates reachable at all: the diff is the API server's.** Every Application here syncs with `ServerSideApply`, and the cluster runs mutating webhooks — CNPG defaults a `Cluster`, Kyverno mutates pods — so the live object legitimately carries fields no chart rendered. A client-side diff calls that drift permanently, the Application never reports Synced, and because a tier is Healthy only when every Application it generated is **Synced and Healthy**, one such resource holds its wave shut and no later tier ever runs. `controller.diff.server.side` diffs against the API server's dry-run instead. The symptom without it is an Application reporting OutOfSync while `argocd app diff` prints nothing.

**What makes the waves real gates:** default ApplicationSet health reflects only successful templating. A custom health check on the `ApplicationSet` kind walks `.status.resources` and reports Progressing until every generated Application is Synced and Healthy, so wave 3 blocks until waves 0 through 2 are up.

A companion health check on the CNPG `Cluster` kind makes the data-to-core gate honest — without it Argo reports the unknown CRD Healthy the instant it applies, and core starts against a Postgres that is not ready. Charts within a tier are still concurrent and must tolerate that.

Each service Application layers the environment's `shared.yaml` before that service's own values file, so a setting true of every service in an environment — the registry's pull credential, for one — is declared once instead of per service. A default repeated eight times is a default one service will eventually be missing.

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

`manualSync: true` on the AppProject's sync window permits an ad-hoc `argocd app sync` outside it for incident response.

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
- **Single-repo deploy state mixes image-bump PRs with feature PRs.** Mitigated by a title prefix and a separate code owner on the GitOps tree, and bounded by the split deferred above.
- **Staging batches by window**, so merge-by-merge behaviour is not observable there. Accepted — it is the explicit goal, and a manual sync is available when the next window is too far away.
- **A bad merge sits on `master` until the next staging window.** Mitigated by full CI on PRs and by dev continuously running `master`.
- **Argo CD itself can fail.** Mitigated by an HA install in production. Downtime blocks new syncs; running workloads are unaffected.
- **Sync waves gate on custom health checks we maintain.** A bug there produces a false ordering guarantee, which is worse than none. Mitigated by the checks being small, committed, and exercised on every local full-tier bring-up.

## Rules

- Argo CD is the only mechanism that applies manifests to a cluster. `kubectl apply` is permitted only for the one-time bootstrap step.
- Every backend service is deployed through the shared chart with per-env values files; platform components have one chart each. `(CI: lint:service-contract)`
- Environment differences live in values files, never in chart logic conditioned on the environment name.
- An image is built once and promoted by updating values files. Rebuilding for another environment is not done. `(CI: ci:publish)`
- Promotion to dev and staging is automatic on merge, with cadence enforced by sync windows. Promotion to production is automatic on a release tag and pins by digest.
- No environment is deployed by hand-opening a values-bump PR.
- Argo CD Image Updater and similar auto-promoters are not used.
- Production platform syncs are manual with `selfHeal=false`; production services sync automatically with `selfHeal=true`.
- Every Application and ApplicationSet carries a `retry` policy.
- Secret values never appear in git; manifests carry SOPS-encrypted files or references to the Secrets they produce.
