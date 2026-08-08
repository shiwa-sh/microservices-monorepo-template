# ADR-0205: Environment Parity

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0202](0202-secrets.md), [ADR-0300](0300-data.md), [ADR-0500](0500-observability.md), [ADR-0600](0600-local-development-loop.md)

## Context

[ADR-0200](0200-cluster-topology.md) commits to parity at the artifact layer: the same charts and manifests apply everywhere, and the distribution differs only in lifecycle. This ADR fixes the boundary precisely — **what may differ between an engineer's laptop and production, and what may never differ.** How the local tiers are built is [ADR-0600](0600-local-development-loop.md).

## Decision drivers

1. **Parity is at the artifact layer.** Same charts, same images, same code, same commands. Divergence is a per-env value, never a forked manifest or chart logic branching on environment.
2. **Composition over forks.** A new environment is a values selection, not a script.
3. **Uphold the ADRs parity would otherwise erode** — [ADR-0200](0200-cluster-topology.md)'s external bucket and off-cluster backups in production, and [ADR-0202](0202-secrets.md)'s single secret mechanism everywhere.
4. **A parity claim must be checkable**, so what may differ is enumerated rather than judged case by case.

## Considered options

| Option | Divergence expressed as | Why not |
| --- | --- | --- |
| **One chart set, per-env values overlays** | `infra/gitops/platform/<env>/values.yaml` | **Chosen** — the divergence surface is one file per environment, diffable and reviewable |
| Per-environment chart forks | separate chart directories | Every upstream change is applied N times, and drift is invisible until an environment breaks |
| Chart logic branching on environment | `{{ if eq .Values.env "prod" }}` | Moves the divergence into templates where it cannot be reviewed as configuration, and makes the production path the least-exercised one |
| A different stack locally (Compose) | a parallel definition | A second definition of the system that drifts silently. The local tier would validate something production does not run |

## Decision

### One chart set, per-env values

Every environment — **local, dev, staging, prod** — deploys the same charts under `infra/helm/platform/*` and `infra/helm/service`. The only sanctioned divergence is a per-env values overlay, with local a first-class member.

A divergence that cannot be expressed as a value — a different chart, or a hand-written Deployment standing in for an operator-managed component — is permitted **only** in the inner-loop tier ([ADR-0600](0600-local-development-loop.md)), and never silently.

**The contract never differs in any tier.** The Kubernetes API, the service chart, the service images, and the env contract (`DATABASE_URL`, `TEMPORAL_HOST_PORT`, the OTLP endpoint, OpenFGA) are identical everywhere. What may differ is *scale*, and — in the inner loop only — the *implementation behind a contract*.

### May differ, must not differ

| Concern | May differ | Must not differ |
| --- | --- | --- |
| Kubernetes distribution | ephemeral locally, persistent when deployed | the charts and manifests applied |
| Scale | replicas, storage size, anti-affinity, HPA | which components the full tier runs |
| Data-tier implementation | inner-loop stand-ins against the real charts | the wire contract, and the Postgres major version |
| Object storage provider | MinIO in non-prod, external bucket in prod | the S3 API contract |
| TLS issuer | a local CA against Let's Encrypt | cert-manager as the mechanism, and that certificates are **verified**, never bypassed |
| Secret plaintext | throwaway locally, real when deployed | SOPS as the decrypt mechanism |
| Domain | `*.localtest.me` against `*.<env>.<project-domain>` | the routing and edge shape |

### Distribution lifecycle

The local distribution and the deployed one run identical charts, so the choice is lifecycle, not architecture.

| Tier | Lifecycle |
| --- | --- |
| local, both loops | per-engineer, recreated freely |
| CI e2e and label-gated PR preview | ephemeral, created and destroyed per run |
| dev, staging, prod | persistent ([ADR-0200](0200-cluster-topology.md)) |

The full-platform local tier and the CI preview tier are the **same configuration**, so investment in one is investment in the other. Deployed environments stay persistent: they survive reboots and hold real storage and backups.

### Object storage

The interface is the S3 API in every environment; only the provider differs by values.

| Environment | Provider |
| --- | --- |
| local, dev, staging | in-cluster MinIO (`infra/helm/platform/minio`) |
| prod | an external S3-compatible bucket |

In-cluster object storage in production would co-locate data and its backups on the same nodes, defeating disaster recovery ([ADR-0200](0200-cluster-topology.md)). CNPG backups target the external bucket in production; in non-prod they may target in-cluster MinIO, where backups are convenience rather than a recovery guarantee. The per-env delta is the S3 endpoint and whether the MinIO chart is enabled.

### Secrets

[ADR-0202](0202-secrets.md) requires one secret mechanism in every environment, and local is no exception: the same SOPS path with a committed, well-known **local** age key. It is safe precisely because the only values it decrypts are throwaway credentials. The decrypt mechanism is identical to production; only the plaintext differs.

## Consequences

### Positive

- The divergence surface is one values file per environment, reviewable in a diff.
- A chart change is exercised in the full local tier before it reaches a deployed environment.
- New environments are values selections, so adding one is an overlay rather than a script.
- The production path is the most-exercised path, because every tier runs it.

### Negative / Risks

- **The inner loop's stand-ins are a sanctioned parity gap.** Bounded to implementation behind an unchanged wire contract, and never extended to the full tier.
- **A single local node is not a production topology.** Saturation shapes transfer; absolute ceilings do not ([ADR-0601](0601-testing-strategy.md)).
- **Non-prod backups are not recovery guarantees.** Stated here so the MinIO convenience is never mistaken for one.
- **Local secrets are committed.** Safe only while the local age key decrypts nothing real; a real credential in a local secret file is a leak, not a shortcut.

## Rules

- Every environment deploys the same charts. The only sanctioned divergence is a per-env values overlay. `(review-only)`
- Chart templates do not branch on environment name. A difference that cannot be expressed as a value is a defect outside the inner-loop tier. `(review-only)`
- The Kubernetes API, service chart, service images, and env contract are identical in every tier. `(review-only)`
- Object storage is the S3 API everywhere; production uses an external bucket, and in-cluster object storage is not run in production. `(review-only)`
- Backups are off-cluster and mandatory in production ([ADR-0200](0200-cluster-topology.md)). Non-prod backups are convenience and are never cited as a recovery guarantee. `(review-only)`
- SOPS is the secret mechanism in every environment, including local. `(CI: lint:secrets)`
- Certificates are issued by cert-manager in every environment and are verified, never bypassed. `(review-only)`
