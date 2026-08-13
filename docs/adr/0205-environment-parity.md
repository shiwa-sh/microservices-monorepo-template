# ADR-0205: Environment Parity

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0003](0003-naming-and-identifiers.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0202](0202-secrets.md), [ADR-0300](0300-data.md), [ADR-0500](0500-observability.md), [ADR-0600](0600-local-development-loop.md), [ADR-0601](0601-testing-strategy.md)
- **Decides:** Every environment deploys the same charts, and the only sanctioned divergence is a per-environment values overlay.

## Context

[ADR-0200](0200-cluster-topology.md) commits to parity at the artifact layer: the same charts and manifests apply everywhere, and the distribution differs only in lifecycle. This ADR fixes the boundary precisely — **what may differ between an engineer's laptop and production, and what may never differ.** How the local tiers are built is [ADR-0600](0600-local-development-loop.md).

## Decision drivers

1. **The production path is the most-exercised path.** Whatever runs in production must be what every other tier runs, or the tiers below validate something production does not do.
2. **A new environment costs a selection, not a script.**
3. **The divergence surface is enumerable**, so what may differ is a list rather than a case-by-case judgement.
4. **Drift is visible before it breaks something**, not at the moment an environment diverges in production.

## Considered options

| Option | Divergence expressed as | Exercises the production path elsewhere | Cost of an upstream change | Verdict |
| --- | --- | --- | --- | --- |
| **One chart set, per-env values overlays** | one values file per environment | yes — every tier renders the same templates | once | **Chosen.** The divergence surface is one diffable file per environment *(reasoned)* |
| Kustomize overlays on a shared base | a base plus a patch directory per environment | yes | once, plus re-checking that each patch still matches the base | Patches match the base structurally, so an upstream change can silently no-op one and the divergence becomes invisible. [ADR-0201](0201-gitops.md) rejects the templating layer for the same reason |
| Per-environment chart forks | separate chart directories | no — each fork is exercised only by its own environment | once per fork | Drift is invisible until an environment breaks |
| Chart logic branching on environment | `{{ if eq .Values.env "prod" }}` | **no — the production branch is the least-exercised code in the repo** | once, and every branch re-reasoned | Moves divergence into templates, where it cannot be reviewed as configuration |
| A different stack locally, such as Compose | a parallel definition | no | twice, in two languages | A second definition of the system that drifts silently. The local tier would validate something production does not run |

## Decision

### One chart set, per-env values

Every environment — **local, dev, staging, prod** — deploys the same charts under `infra/helm/platform/*` and `infra/helm/service`. The only sanctioned divergence is a per-env values overlay, with local a first-class member.

A divergence that cannot be expressed as a value — a different chart, or a hand-written Deployment standing in for an operator-managed component — is permitted **only** in the inner-loop tier ([ADR-0600](0600-local-development-loop.md)), and never silently.

**The contract never differs in any tier.** The Kubernetes API, the service chart, the service images, and the env contract (`DATABASE_URL`, `TEMPORAL_HOST_PORT`, the OTLP endpoint, OpenFGA) are identical everywhere. What may differ is *scale*, and — in the inner loop only — the *implementation behind a contract*.

### May differ, must not differ

| Concern | May differ | Must not differ |
| --- | --- | --- |
| Kubernetes distribution | ephemeral locally, persistent when deployed; node count in the inner loop, and how that node is configured rather than what it runs ([ADR-0600](0600-local-development-loop.md)) | the charts and manifests applied, the distribution and its datastore in every tier, and the machine config from the full tier upward |
| Scale | replicas, storage size, anti-affinity, HPA | which components the full tier runs |
| Data-tier implementation | inner-loop stand-ins against the real charts | the wire contract, and the Postgres major version |
| Object storage placement | in-cluster in non-prod, outside the cluster in prod | the implementation, and the S3 API contract |
| TLS issuer | a local CA against Let's Encrypt | cert-manager as the mechanism, and that certificates are **verified**, never bypassed |
| Secret plaintext | throwaway locally, real when deployed | SOPS as the decrypt mechanism |
| Domain | `*.localtest.me` against `*.<env>.<project-domain>` | the routing and edge shape |

The **must not differ** column is [12-Factor X](https://12factor.net/dev-prod-parity) — *"resists the urge to use different backing services between development and production"* — made enumerable. The **may differ** column is the deliberate scope: the factor is silent on scale, and an inner-loop stand-in behind an identical wire contract is not a different backing service.

### Distribution lifecycle

| Tier | Distribution | Lifecycle |
| --- | --- | --- |
| local inner loop | upstream Kubernetes on etcd, on one node ([ADR-0600](0600-local-development-loop.md)) | per-engineer, recreated freely |
| local full platform | as [ADR-0200](0200-cluster-topology.md) decides, from the same machine config, provisioned in containers | per-engineer, recreated freely |
| CI e2e and label-gated PR preview | as the local full platform | ephemeral, created and destroyed per run |
| dev, staging, prod | as [ADR-0200](0200-cluster-topology.md) decides, provisioned on hosts | persistent |

**Distribution parity is a floor, and the floor is the full tier.** From it upward the machine config is fixed and the choice is lifecycle, not architecture. The inner loop sits one step below: it runs the same Kubernetes on the same datastore with the same datapath, and differs only in node count and in reaching that state by configuration rather than by machine config. Its sanctioned divergence is spent on the stand-ins behind a wire contract, not on the cluster underneath them.

The full-platform local tier and the CI preview tier are the **same configuration**, so investment in one is investment in the other, and neither is a second chance to catch what the other misses. Deployed environments stay persistent: they survive reboots and hold real storage and backups.

### The ephemeral PR environment

A preview is the full-platform tier ([ADR-0600](0600-local-development-loop.md)) brought up in CI against one pull request's images. It is a lifecycle of the tier already decided rather than a fourth kind of environment, which is why it lives here and not in an ADR of its own.

| Concern | Decision |
| --- | --- |
| Trigger | a label on the pull request. Bring-up costs minutes of runner time, so it is opt-in per PR rather than per push ([ADR-0601](0601-testing-strategy.md)) |
| Configuration | the same charts and the same `local` values overlay the full tier runs, with images from the PR's build |
| Identity | the committed deterministic test identities, never a copy of any deployed environment's data |
| Lifetime | destroyed when the run ends or the pull request closes. Nothing survives it, and nothing outside it depends on it |
| Naming | the environment slug derives from the pull request number ([ADR-0003](0003-naming-and-identifiers.md)), so two previews cannot collide |

**A preview holds no real data and no real credential.** It is a throwaway cluster, so it takes the local tier's secret path — the committed local age key ([ADR-0202](0202-secrets.md)) — rather than any deployed environment's. An environment that borrowed production data or production secrets to be realistic would be a production system with a pull-request lifetime.

**Previews are not a deployment target.** No preview is promoted, and nothing is released from one. It is a test fixture with a URL.

### Object storage

**Parity here is the implementation, not the API alone.** One store runs in every environment, and the sole delta is placement: in-cluster in non-prod, outside the cluster in production, for the failure-domain reason [ADR-0200](0200-cluster-topology.md) states.

Expressed as values, that delta is the S3 endpoint and whether the in-cluster chart is enabled. CNPG backups target the production bucket; in non-prod they target the in-cluster instance, where a backup is convenience rather than a recovery guarantee.

### Secrets

[ADR-0202](0202-secrets.md) requires one secret mechanism in every environment, and local is no exception: the same SOPS path with a committed, well-known **local** age key. It is safe precisely because the only values it decrypts are throwaway credentials. The decrypt mechanism is identical to production; only the plaintext differs.

## Consequences

### Positive

- The divergence surface is one values file per environment, reviewable in a diff.
- A chart change is exercised in the full local tier before it reaches a deployed environment.
- New environments are values selections, so adding one is an overlay rather than a script.
- The production path is the most-exercised path, because every tier runs it.

### Negative / Risks

- **The inner loop's stand-ins are a sanctioned parity gap.** Bounded to implementation behind an unchanged wire contract, enumerated rather than argued, and never extended to the full tier.
- **The full tier is the last tier before a deployed environment.** CI and the preview share its configuration, so a divergence carried there is examined nowhere. That is the reason distribution parity starts at the full tier rather than one tier higher.
- **A single local node is not a production topology.** Saturation shapes transfer; absolute ceilings do not ([ADR-0601](0601-testing-strategy.md)).
- **Non-prod backups are not recovery guarantees.** Stated here so the in-cluster convenience is never mistaken for one.
- **Local secrets are committed.** Safe only while the local age key decrypts nothing real; a real credential in a local secret file is a leak, not a shortcut.

## Rules

- Every environment deploys the same charts. The only sanctioned divergence is a per-env values overlay.
- Every tier runs the distribution and datastore [ADR-0200](0200-cluster-topology.md) decides. From the full local platform upward the machine config is the same too, and a tier differs from a deployed environment only in lifecycle and in how its nodes are provisioned. The inner loop differs in node count alone ([ADR-0600](0600-local-development-loop.md)).
- Chart templates do not branch on environment name. A difference that cannot be expressed as a value is a defect outside the inner-loop tier.
- The Kubernetes API, service chart, service images, and env contract are identical in every tier.
- Object storage is one implementation in every environment ([ADR-0207](0207-cluster-storage.md)). Production runs it outside the cluster, and no store holding production data runs on the cluster it serves.
- Backups are off-cluster and mandatory in production ([ADR-0207](0207-cluster-storage.md)). Non-prod backups are convenience and are never cited as a recovery guarantee.
- SOPS is the secret mechanism in every environment, including local.
- A PR preview is the full-platform tier at a pull request's images, label-gated, and destroyed with the run. Its slug derives from the pull request number.
- A preview uses the local secret path and the committed test identities. No deployed environment's data or credentials are copied into one.
- Nothing is promoted or released from a preview.
- Certificates are issued by cert-manager over ACME in every environment and are verified, never bypassed. `(ref: RFC 8555)`
