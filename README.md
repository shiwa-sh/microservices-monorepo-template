# Microservices Monorepo Template

A GitHub template for new microservice platforms. Every reusable piece is pre-decided
in the [ADRs](docs/adr) so a new project starts at "build features" rather than "pick stacks."

> Read [ADR-0000](docs/adr/0000-platform-foundations.md) first — it pins the thesis, the
> principles, and the vocabulary every other ADR references.

## Stack at a glance

| Concern              | Tool                                                      | ADR                                                                              |
|----------------------|-----------------------------------------------------------|----------------------------------------------------------------------------------|
| Backend language     | Go                                                        | [0001](docs/adr/0001-language-and-runtime.md)                                    |
| Frontend             | Next.js + TypeScript (one app, route groups)              | [0001](docs/adr/0001-language-and-runtime.md), [0002](docs/adr/0002-monorepo.md) |
| Task runner          | `mise` (root `.mise.toml`)                                | [0002](docs/adr/0002-monorepo.md)                                                |
| Go workspace         | Single repo-wide `go.mod`                                 | [0002](docs/adr/0002-monorepo.md)                                                |
| TS workspace         | Bun workspaces                                            | [0002](docs/adr/0002-monorepo.md)                                                |
| CI                   | GitHub Actions                                            | [0002](docs/adr/0002-monorepo.md)                                                |
| Cluster (prod)       | k3s on compute instances (Ansible; Terraform per-project) | [0003](docs/adr/0003-cluster-topology.md)                                        |
| Cluster (local)      | k3d, same Helm charts as prod                             | [0003](docs/adr/0003-cluster-topology.md)                                        |
| Ingress / TLS        | Traefik + cert-manager                                    | [0003](docs/adr/0003-cluster-topology.md)                                        |
| GitOps               | ArgoCD + ApplicationSets                                  | [0004](docs/adr/0004-gitops.md)                                                  |
| Secrets              | SOPS + age                                                | [0005](docs/adr/0005-secrets.md)                                                 |
| Workflows            | Self-hosted Temporal                                      | [0006](docs/adr/0006-temporal.md)                                                |
| Database             | PostgreSQL via CNPG, sqlc, dbmate, sqruff                 | [0007](docs/adr/0007-data.md)                                                    |
| API contract         | OpenAPI 3.1 (ogen + openapi-typescript)                   | [0008](docs/adr/0008-api-contracts.md)                                           |
| Edge                 | Traefik + Ory Oathkeeper                                  | [0009](docs/adr/0009-api-gateway.md)                                             |
| Auth                 | Ory Kratos + Oathkeeper + SpiceDB (Hydra per-flag)        | [0010](docs/adr/0010-auth.md)                                                    |
| Observability        | OpenTelemetry → Grafana LGTM (monolithic)                 | [0011](docs/adr/0011-observability.md)                                           |
| Internal admin       | Lowdefy (YAML pages)                                      | [0012](docs/adr/0012-internal-admin.md)                                          |
| Release / versioning | Conventional Commits + cocogitto, tag per product         | [0013](docs/adr/0013-release-and-versioning.md)                                  |

## Why these choices — and how to adapt them

Every decision above is tuned for one **target scale** ([ADR-0000](docs/adr/0000-platform-foundations.md)):
**~100 services, a small team (3–8 engineers), self-hosted Kubernetes.** That thesis is the
load-bearing assumption — if you adopt this template under different conditions, some ADRs should
move. The thesis bundles two *independent* levers that pull different decisions:

- **Number of services (N)** sets **how much machinery you build.** Per-service cost, affected
  detection, the single `go.mod`, ApplicationSet fan-out, the shared service chart, codegen, and
  naming discipline all scale with N. More services → more automation and standardization; fewer →
  strip it.
- **Team size (T)** sets your **operational-surface budget and isolation needs.** Self-host vs
  managed, how many distinct tools you can run, "one way to do things," module/repo isolation,
  ArgoCD RBAC, and governance formality all scale with T. Smaller team → fewer moving parts,
  managed-by-default, standardization by necessity; larger team → more components and specialists
  affordable, but you need isolation and heavier governance.

The single most useful number is the **ratio N/T (services per engineer).** This template targets
~12–30 services/engineer, which is very high — that ratio, not the absolute 100, is what justifies
the "platform as a product, automate and standardize everything" stance. (Service count tracks the
number of independent *teams*, not features — Conway's law — so a small team running many services is
deliberately compensating for an abnormally high N/T with tooling.)

### If your conditions differ

| You have…                     | Move toward…                                                                                                              |
|-------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| **Fewer services** (5–20)     | Drop `tools/affected/` (run everything), relax codegen/fan-out, reconsider Go vs a velocity-first language.               |
| **Many more services** (300+) | Split the `go.mod`, adopt Bazel/Nx + remote build cache, shard ArgoCD, multiple DB clusters, revisit a mesh.              |
| **A smaller team** (1–2)      | Trade money for time: managed K8s/Postgres/Temporal/auth/observability. Keep standardization *tighter*.                   |
| **A bigger team** (20–50+)    | Loosen "one way" toward a golden path, add module/repo isolation, ArgoCD RBAC, RFC governance, maybe an event bus / mesh. |

The 2×2 of these (small-vs-big team × few-vs-many services) gives four distinct cultures. The hardest
and the one this template is built for is **small team + many services**: you must *simultaneously*
maximize automation and minimize operational surface. The opposite corner, **big team + few
services**, optimizes for depth and autonomy per service instead, and can afford polyglot and
per-service infra.

### On self-hosting

Self-host is a **firm choice** for this template, not a default we expect everyone to flip — many
platforms need infrastructure they fully own and control (data residency, internal-infra
requirements, cost predictability at scale). It is, however, the decision most sensitive to **team
size**: the per-service-cost math makes self-host *cheaper* as N grows, but operating ~15 stateful
components is what a 3–8 person team can absorb and a 1–2 person team cannot. If you adopt this
template with a very small team and no infra appetite, self-host is the first thing to reconsider —
swapping to managed K8s/Postgres/Temporal/auth/observability reclaims the operational budget those
people don't have. Everything else in the ADRs survives that swap.

### Building it solo, with LLMs

If a single person builds a platform like this with heavy LLM assistance, the binding constraint
shifts. LLMs make *authoring* cheap — boilerplate, committed codegen, verbose Go, repetitive values
files all stop mattering — so those costs the ADRs guard against largely dissolve. What LLMs do *not*
cheapen is **operational surface** (a solo operator still carries the pager) and **coherent review at
volume**. So solo + LLM pushes three ways: standardization and machine-checkable guardrails (the
ADR **Rules** sections, strong typing, drift-checked codegen) become *more* valuable as the agent
control plane; self-host should flip to managed; and you'd likely build a **modular monolith or a
handful of services** rather than 100, since service count buys team autonomy a solo dev doesn't
need. Pick tools by training-data density and verifiability — a criterion these ADRs already lean on
(OpenAPI as source of truth, Lowdefy chosen partly as "LLM-friendly").

## Getting started

```sh
# One-time
curl https://mise.run | sh
mise install                  # pinned tools from .mise.toml
mise run setup                # install git hooks
age-keygen -o ~/.config/sops/age/keys.txt   # SOPS private key (ADR-0005)

# Local cluster
mise run cluster:lite         # k3d cluster + local deps (Postgres, Temporal, SpiceDB)

# Inner loop on a single service
cd services/catalog
mise run run                  # http server
mise run worker               # temporal worker
```

## Layout

```text
services/<name>/    — backend service (Go, OpenAPI, server + worker, sqlc, migrations)
apps/frontend/      — single Next.js app (route groups: landing|panel|admin|devportal)
apps/admin/         — Lowdefy YAML config for internal admin
libs/go/<name>/     — shared Go packages
libs/ts/<name>/     — shared TS packages
libs/{go,ts}/sdks/  — generated OpenAPI clients (committed, drift-checked)
infra/              — terraform, helm, gitops, ansible, gateway, observability, auth
tools/              — repo-local Go programs (affected detection, auth-conformance, …)
docs/adr/           — architectural decisions
```

The conventions above are not negotiable per-service — they're load-bearing for ADRs
[0002](docs/adr/0002-monorepo.md), [0007](docs/adr/0007-data.md), and
[0008](docs/adr/0008-api-contracts.md). Deviations require a new ADR.

## Worked example: the shop

The template ships with a tiny "shop" example exercising every tool from the ADRs once:

| Service   | Demonstrates                                                   |
|-----------|----------------------------------------------------------------|
| `orgs`    | Kratos identity + B2B multi-tenancy + SpiceDB ReBAC            |
| `catalog` | Plain CRUD: OpenAPI handler, sqlc, migrations, observability   |
| `orders`  | Temporal checkout saga calling `catalog` + `payment` over HTTP |
| `payment` | Child workflow with idempotency + compensation                 |

Each service is the minimum that proves a mechanism is wired — not a realistic
product. Build real services from `services/_template/`.
