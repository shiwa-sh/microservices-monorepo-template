# Microservices Monorepo Template

A GitHub template for new microservice platforms. Every reusable piece is pre-decided
in the [ADRs](docs/adr/) so a new project starts at "build features" rather than "pick stacks."

> Read [ADR-0000](docs/adr/0000-platform-foundations.md) first — it pins the thesis, the
> principles, and the vocabulary every other ADR references.

## Stack at a glance

| Concern             | Tool                                              | ADR |
|---------------------|---------------------------------------------------|-----|
| Backend language    | Go                                                | [0001](docs/adr/0001-language-and-runtime.md) |
| Frontend            | Next.js + TypeScript (one app, route groups)      | [0001](docs/adr/0001-language-and-runtime.md), [0002](docs/adr/0002-monorepo.md) |
| Task runner         | `mise` (root `.mise.toml`)                        | [0002](docs/adr/0002-monorepo.md) |
| Go workspace        | Single repo-wide `go.mod`                         | [0002](docs/adr/0002-monorepo.md) |
| TS workspace        | Bun workspaces                                    | [0002](docs/adr/0002-monorepo.md) |
| CI                  | GitHub Actions                                    | [0002](docs/adr/0002-monorepo.md) |
| Cluster (prod)      | k3s on Hetzner Cloud (Terraform + Ansible)        | [0003](docs/adr/0003-cluster-topology.md) |
| Cluster (local)     | k3d, same Helm charts as prod                     | [0003](docs/adr/0003-cluster-topology.md) |
| Ingress / TLS       | Traefik + cert-manager                            | [0003](docs/adr/0003-cluster-topology.md) |
| GitOps              | ArgoCD + ApplicationSets                          | [0004](docs/adr/0004-gitops.md) |
| Secrets             | SOPS + age                                        | [0005](docs/adr/0005-secrets.md) |
| Workflows           | Self-hosted Temporal                              | [0006](docs/adr/0006-temporal.md) |
| Database            | PostgreSQL via CNPG, sqlc, dbmate, sqlfluff       | [0007](docs/adr/0007-data.md) |
| API contract        | OpenAPI 3.1 (oapi-codegen + openapi-typescript)   | [0008](docs/adr/0008-api-contracts.md) |
| API gateway         | Tyk Gateway OSS                                   | [0009](docs/adr/0009-api-gateway.md) |
| Auth                | Ory Kratos + Hydra + SpiceDB                      | [0010](docs/adr/0010-auth.md) |
| Observability       | OpenTelemetry → Grafana LGTM + Pyroscope          | [0011](docs/adr/0011-observability.md) |
| Internal admin      | Lowdefy (YAML pages)                              | [0012](docs/adr/0012-internal-admin.md) |
| Release / versioning| Conventional Commits + cocogitto, tag per product | [0013](docs/adr/0013-release-and-versioning.md) |

## Getting started

```sh
# One-time
curl https://mise.run | sh
mise install                  # pinned tools from .mise.toml
mise run setup                # install git hooks
age-keygen -o ~/.config/sops/age/keys.txt   # SOPS private key (ADR-0005)

# Local cluster
mise run dev:up               # full profile (everything, ~90s)
mise run dev:up -- --minimal  # inner-loop profile (~20s)

# Inner loop on a single service
cd services/catalog
mise run run                  # http server
mise run worker               # temporal worker

# Health check
bash scripts/doctor.sh
```

## Layout

```
services/<name>/    — backend service (Go, OpenAPI, server + worker, sqlc, migrations)
apps/frontend/      — single Next.js app (route groups: landing|panel|admin|devportal)
apps/admin/         — Lowdefy YAML config for internal admin
libs/go/<name>/     — shared Go packages
libs/ts/<name>/     — shared TS packages
libs/sdks/{go,ts}/  — generated OpenAPI clients (committed, drift-checked)
infra/              — terraform, helm, gitops, ansible, gateway, observability, auth
tools/              — repo-local Go programs (affected detection, tyk-gen, …)
docs/adr/           — architectural decisions
```

The conventions above are not negotiable per-service — they're load-bearing for ADRs
[0002](docs/adr/0002-monorepo.md), [0007](docs/adr/0007-data.md), and
[0008](docs/adr/0008-api-contracts.md). Deviations require a new ADR.

## Worked example: the shop

The template ships with a tiny "shop" example exercising every tool from the ADRs once:

| Service   | Demonstrates                                                  |
|-----------|----------------------------------------------------------------|
| `orgs`    | Kratos identity + B2B multi-tenancy + SpiceDB ReBAC            |
| `catalog` | Plain CRUD: OpenAPI handler, sqlc, migrations, observability   |
| `orders`  | Temporal checkout saga calling `catalog` + `payment` over HTTP |
| `payment` | Child workflow with idempotency + compensation                 |

Each service is the minimum that proves a mechanism is wired — not a realistic
product. Build real services from `services/_template/`.
