# Microservices Monorepo Template

A reference architecture for **self-hosted product platforms** — a monorepo where every reusable decision is already made, recorded in an [ADR](docs/adr), and enforced by a machine.

Fork it, generate from it, or read the ADRs and ignore the code. All three are valid uses.

**The stance, in three lines:**

- **The menu is omakase.** The stack is pre-decided. You start at "build features," not "pick tools."
- **A paved road, with off-ramps.** Deviating is allowed and documented — it has to be deliberate.
- **Enforced by machines.** Rules are admission policies, type checks, and drift checks. Review is the fallback, not the mechanism.

---

## Which profile are you?

This template ships one profile and documents three neighbours. Find yourself before reading further.

| Profile | You are here if… | Style | Deployables | Platform components | Platform engineers |
| --- | --- | --- | --- | --- | --- |
| `modular-monolith` | No decomposition force applies (see below) | Modular monolith | 1 | ~4 | 0 |
| `service-based` | Teams block each other on deploys | Service-based | 3–8 | ~8 | ~0.5 |
| `microservices` | Coordination cost dominates engineering cost | Microservices | 15+ | ~15 | 1–2 |
| **`sovereign`** ← **this repo** | Self-hosting is *binding*, not preferred | Microservices | 15+ | **a couple of dozen** | **2–3** |

> **These are not maturity levels.** A modular monolith is a correct *terminal* state for most systems, not a waypoint on the road to microservices. Higher rows are not better — they are more expensive answers to pressures you may not have. This follows Richards & Ford's treatment of architecture styles as **risk profiles rated against characteristics**, and deliberately rejects the numbered-ladder genre (CMMI, and the maturity models that followed it) — a framing DORA/*Accelerate* also argues against.

---

## What forces decomposition

Service count is an **outcome**, not a target. Each force independently justifies a boundary. **If none apply, build a modular monolith.**

| # | Force | Test |
| --- | --- | --- |
| 1 | Independent deploy cadence | Two parts must ship without coordinating a release |
| 2 | Number of teams | Teams block each other. Conway's law — teams, not headcount, not features |
| 3 | Failure isolation | One part failing must not take another down, as a *stated requirement* |
| 4 | Resource heterogeneity | Genuinely different scaling profiles (CPU vs IO vs memory) |
| 5 | Technology heterogeneity | A part must run on another runtime (ML in Python, chain code in Rust) |
| 6 | Compliance boundary | Data residency or audit scope cheaper to enforce structurally than by policy |

Count the ones that genuinely apply. That is your service count.

---

## The three characteristics

Product category ("fintech", "B2B SaaS", "marketplace") determines almost nothing — two products in one category routinely sit at opposite ends of every axis. These three determine nearly everything:

| Axis | Question | Range | This repo |
| --- | --- | --- | --- |
| **A — Decomposition pressure** | Must parts ship, scale, or fail independently? | monolith → many services | high |
| **B — Operational sovereignty** | Who is permitted to run it? | managed → hybrid → self-hosted | **maximum** |
| **C — Correctness stakes** | What does a wrong answer cost? | cosmetic → transactional → regulated | high |

**Axis B is the distinguishing choice.** Most reference architectures at this position on A and C assume managed services underneath. This one does not — and that single decision generates more of the machinery here than A and C combined.

---

## Need vs. capacity

> **Need determines what you build. Capacity determines what you can run. The gap is your risk.**

Team size is a **budget, not a design driver**. It never tells you to build thirty services — only whether you can operate what you have.

What `sovereign` costs to run:

- **An always-on platform floor of a couple of dozen components** — gateway, GitOps, identity, authz, workflow, data, storage, observability, registry, forge, outbound mail. [`docs/operational-surface.md`](docs/operational-surface.md) is the inventory
- **2–3 engineers whose primary job is the platform** — not product engineers on rotation. This is the most common way a self-hosted platform rots
- Component count is **fixed**. It does not shrink with fewer services

A consequence worth internalising: **fewer services makes the platform proportionally heavier.** At many services the application-to-platform workload ratio is comfortable; at a handful they are roughly 1:1. Fewer services *strengthens* the case for austerity — it does not relax it.

---

## What you don't need on day one

Not everything here ships at once. Capabilities are deferred behind a **hard trigger** — an *observable* condition, never "when we grow." Anything deferred without a trigger is "temporary," which [ADR-0000](docs/adr/0000-platform-foundations.md) forbids outright.

Deferral is only safe when a **seam** exists — a pre-built slot where the capability drops in without restructuring. The distinction matters more than the list:

- ✅ **Seam exists** — adoption is additive. Deferring costs nothing but the wait.
- ⚠️ **No full seam** — adoption is structural. This is a *bet*, not a deferral. Adopt earlier, or accept a rewrite later.

| Capability | Trigger | Seam | Tier |
| --- | --- | --- | --- |
| Ory Hydra (OAuth2 server) | A public API or external machine client exists | ✅ flag-gated; internal request shape unchanged | Opt-in |
| Mimir (metrics at scale) | Prometheus needs multi-tenancy, HA, or long retention | ✅ same query API — dashboards and alerts port unchanged | Scale |
| Tail sampling | Trace volume makes head sampling lossy | ✅ services only ever emit to `localhost:4317` | Scale |
| Loki / Tempo microservices mode | A single backend's ingest volume demands it | ✅ same storage, same API | Scale |
| Rust or Python service | *Measured* Go inadequacy, or a runtime-native ecosystem need | ✅ contracts are HTTP/OpenAPI, not shared code | Opt-in |
| i18n (`next-intl`) | A second locale on the roadmap | ⚠️ retrofitting strings across routes is real work | Opt-in |
| Storybook | A second frontend app, or a design-system maintainer | ⚠️ | Opt-in |
| Service mesh | A measured need Cilium + WireGuard cannot meet | ⚠️ partial — mTLS is covered, per-request policy is not | Scale |
| Alert routing / on-call | An on-call rota exists | ⚠️ **no self-hosted escalation layer chosen yet** | Opt-in |

Deferring on purpose, with the trigger written down, is the **last responsible moment** applied to architecture — not the same thing as not having decided.

---

## When not to use this

- **No decomposition force applies** → `modular-monolith`. This is the common case.
- **You cannot staff 2–3 platform engineers** → the floor does not shrink to fit. Move down axis B.
- **Managed services are acceptable to you** → much of this repo solves a problem you do not have.
- **Correctness stakes are low** → the typing, contract, and policy discipline here is priced for money-handling systems and will read as friction.

---

## Lower on the sovereignty axis

Axis B is an axis, not a virtue test. **A smaller team should sit lower on it deliberately** — that is a better answer than adopting the self-hosted floor and hoping.

Axis B is also the axis this template is **least coupled to**: moving down it swaps *operators*, not architecture. Unchanged by a managed swap — the ADRs, OpenAPI contracts and codegen, the service template, the Kustomize tree, policy-as-admission, repo layout, release flow, and the local dev loop.

Swap in this order — ranked by capacity returned per unit of sovereignty conceded:

| # | Self-hosted here | Swap to | Why this rank |
| --- | --- | --- | --- |
| 1 | Outbound email (MTA) | Any transactional email provider | Deliverability is *reputational*, not technical. The one cost engineering cannot retire |
| 2 | Alert routing / on-call | A hosted paging service | No mature self-hosted escalation layer exists |
| 3 | PostgreSQL (CNPG) | Managed Postgres | Highest operational risk per engineering hour |
| 4 | Object storage (Rook-Ceph) | Any S3-compatible service | Removes a distributed storage system *and* the off-cluster backup problem |
| 5 | Observability (Grafana stack) | A hosted backend | Four components → one. OTel instrumentation is unchanged |
| 6 | Temporal | Temporal Cloud | Workflow code identical; only the connection target changes |
| 7 | Forge + CI (Forgejo) | A hosted forge | Cheap either way — CI logic lives in `mise run ci:*` |
| 8 | Registry (Harbor) | A hosted registry | Verify Cosign policy + scanning parity first |
| 9 | Identity (Ory) | A hosted IdP | Late: identity data is the most painful thing to migrate twice |
| 10 | Kubernetes | Managed Kubernetes | Keep the API, drop the substrate |

**Hybrid is legitimate and common:** self-host the orchestrator and stateless platform; take managed Postgres, object storage, email, and paging. Removes most of the pager burden, keeps the parts that usually motivated self-hosting.

---

## How tools are chosen

Ten principles decide every entry in the stack table below. The test for being on this list:

> **A principle earns its place only if it has rejected something we wanted.**

Anything that cannot name a casualty is a virtue statement, not a criterion.

Each principle is **anchored** to an external standard, or explicitly marked **local** where no standard applies. The distinction is deliberate: a borrowed criterion survives re-litigation, a house rule gets re-argued every year, and you should be able to tell which is which at a glance. Full reasoning in [ADR-0000](docs/adr/0000-platform-foundations.md).

**Selection — how a tool gets in:**

| # | Principle | Anchored to | It rejected |
| --- | --- | --- | --- |
| 1 | Configuration lives in the repository, not in the component | [OpenGitOps](https://opengitops.dev/) 1–2, extended one level out | SigNoz, OpenObserve, Coroot, Zitadel |
| 2 | Thinnest viable platform — the always-on floor is the budget | [Team Topologies](https://teamtopologies.com/key-concepts-content/what-is-a-thinnest-viable-platform-tvp) | GitLab CE, Coroot, a second CI system |
| 3 | Operational sovereignty — run it yourself | axis B, above | every managed service |
| 4 | Spend novelty by exit cost, not by taste | [Innovation tokens](https://mcfunley.com/choose-boring-technology) | Garage, SeaweedFS, Encore |
| 5 | One primitive per concern — no parallel mechanisms for one problem | **local** — falls out of principle 2 | Woodpecker, Tekton, Argo Workflows |
| 6 | Two languages, and no ambient runtime | [12-Factor II](https://12factor.net/dependencies) — *"never relies on implicit existence of system-wide packages"*; the two-language cap itself is **local** | Semgrep, sqlfluff (both Python) |
| 7 | No adoption without a recorded comparison | ADR-0002, reserved | *the process gate for all of the above* |

**Construction — how the system is built once a tool is in:**

| # | Principle | Anchored to | It rejected |
| --- | --- | --- | --- |
| 8 | Local–prod parity at the manifest and API layer | [12-Factor X](https://12factor.net/dev-prod-parity) — *"resists the urge to use different backing services between development and production"* | Docker Compose — a second definition of the system |
| 9 | Generated code is committed and drift-checked in CI | **local** — principle 1 applied to generated artifacts | codegen at runtime or at deploy |
| 10 | Service boundaries are HTTP/OpenAPI — never shared code, never a shared database | Fowler, [*IntegrationDatabase*](https://martinfowler.com/bliki/IntegrationDatabase.html) — *"integration databases should be avoided"* | shared libraries and shared schemas as coupling |

Principle 4 is worth reading twice, because it is the one most often misread as conservatism: **be adventurous where abandonment costs days** (CI, query layers, registries, policy engines) **and conservative where it costs months and customer data** (storage, databases, the message bus). The planned adoptions of Talos, Kyverno, and mirrord all sit on the cheap-exit side of that line.

---

## Stack at a glance

Every row is a decision recorded in an ADR, with a comparison against the alternatives: options considered, what each was judged on, why the alternatives lost, and what would reopen it. A tool with no recorded comparison is an assumption, not a decision.

The **Planned change** column names a replacement under consideration but not yet decided in an ADR. Where it is empty, the current column is the whole story.

| Concern | Current decision | ADR | Planned change |
| --- | --- | --- | --- |
| Backend language | Go | [0100](docs/adr/0100-language-and-runtime.md) | |
| Frontend | Next.js + TypeScript on Bun, one app with route groups | [0100](docs/adr/0100-language-and-runtime.md), [0400](docs/adr/0400-frontend.md) | record the framework comparison |
| Design system | Untitled UI on Tailwind v4 | [0400](docs/adr/0400-frontend.md) | |
| Task runner | `mise` | [0101](docs/adr/0101-monorepo.md) | |
| Machines | Debian stable, configured by Ansible | [0200](docs/adr/0200-cluster-topology.md) | Talos Linux |
| Cloud resources | Terraform, per project | [0200](docs/adr/0200-cluster-topology.md) | OpenTofu |
| Cluster | k3s in production, k3d locally | [0200](docs/adr/0200-cluster-topology.md) | kind locally |
| Deploy | Argo CD, the only mechanism; Helm charts, per-env values | [0201](docs/adr/0201-gitops.md) | Kustomize for first-party |
| Network / policy | Cilium + Hubble, WireGuard, default-deny | [0200](docs/adr/0200-cluster-topology.md) | |
| Resource governance | LimitRange, ResourceQuota, PriorityClass, PDB | [0204](docs/adr/0204-resource-management.md) | Kyverno admission |
| Edge | Traefik + Ory Oathkeeper + cert-manager | [0305](docs/adr/0305-edge-auth-and-traffic-policy.md) | |
| Identity / authz | Ory Kratos + OpenFGA; Hydra when a public API exists | [0304](docs/adr/0304-identity-and-authorization.md) | |
| Trust tiers | product apex, `*.ops.<host>` for operator tooling | [0306](docs/adr/0306-trust-tiers-and-urls.md) | |
| Secrets | SOPS + age, decrypted in-cluster by an operator | [0202](docs/adr/0202-secrets.md) | |
| Database | PostgreSQL via CNPG; sqlc, dbmate, sqruff | [0300](docs/adr/0300-data.md) | Atlas for migrations |
| Object storage | external S3 bucket in prod, MinIO in non-prod | [0200](docs/adr/0200-cluster-topology.md), [0205](docs/adr/0205-environment-parity.md) | |
| Workflows | self-hosted Temporal, versioned workers | [0302](docs/adr/0302-temporal.md) | |
| API contract | hand-written OpenAPI 3.1; ogen, openapi-typescript, vacuum | [0303](docs/adr/0303-api-contracts-and-lifecycle.md) | TypeSpec authoring, oasdiff |
| API mocking | Prism, from the committed projection | [0600](docs/adr/0600-local-development-loop.md) | Microcks |
| Observability | OpenTelemetry → Grafana, Loki, Tempo, Prometheus, Pyroscope | [0500](docs/adr/0500-observability.md) | |
| Operator UIs | Grafana funnel, Hubble UI, Headlamp | [0501](docs/adr/0501-operator-uis-and-dashboards.md) | |
| Source control + CI | GitHub Actions, entered through `mise run ci:*` | [0101](docs/adr/0101-monorepo.md) | Forgejo + Forgejo Actions |
| Supply chain | Cosign keyless, syft SBOM, SLSA provenance | [0104](docs/adr/0104-supply-chain-security.md) | Harbor registry, Trivy |
| Testing | Playwright, k6, `go test`, `bun test` | [0601](docs/adr/0601-testing-strategy.md) | Testcontainers, PR environments |
| Internal admin | Lowdefy over the service APIs; pgweb read-only | [0401](docs/adr/0401-internal-admin.md) | |
| Analytics | Faro events into a first-party service | [0700](docs/adr/0700-analytics.md) | |
| Propagation | — | — | Copier + Renovate |

---

## Getting started

```sh
# One-time
curl https://mise.run | sh
mise install                    # pinned toolchain from .mise.toml
mise run setup                  # git hooks
mise run secrets:age            # generate your age key (ADR-0202)

# Local cluster — kind, running the same manifests as production
mise run dev:fe                 # frontend tier: edge + identity + mocks
mise run dev:full               # everything, including observability

# Inner loop on one service — native process, no image build
cd services/catalog
mise run server                 # or: mise run worker
```

Full walkthrough: [`docs/dev-loop.md`](docs/dev-loop.md).

---

## Layout

```text
services/<name>/     — Go service: TypeSpec contract, server, worker, sqlc, migrations
apps/frontend/       — one Next.js app (route groups: landing|panel|devportal)
apps/admin/          — Lowdefy YAML for internal admin
libs/go/<name>/      — shared Go packages (observability, middleware, errors)
libs/{go,ts}/sdks/   — generated OpenAPI clients (committed, drift-checked in CI)
infra/k8s/           — Kustomize base/ components/ overlays/ — one manifest tree, all environments
infra/helm/          — third-party charts only
infra/policy/        — Kyverno policies
infra/talos/         — machine configs
docs/adr/            — the decisions, and why
```

Conventions here are not negotiable per-service — they are load-bearing for [0101](docs/adr/0101-monorepo.md), [0300](docs/adr/0300-data.md), and [0303](docs/adr/0303-api-contracts-and-lifecycle.md). Deviations require a new ADR.

---

## Worked example: the shop

A tiny example exercising every mechanism once — not a realistic product.

| Service | Demonstrates |
| --- | --- |
| `orgs` | Kratos identity + B2B multi-tenancy + OpenFGA ReBAC |
| `catalog` | Plain CRUD: contract, handler, sqlc, migrations, observability |
| `orders` | Temporal checkout saga calling `catalog` + `payment` over HTTP |
| `payment` | Child workflow with idempotency + compensation |

Build real services from `services/_template/`.

---

## How decisions are recorded

- **[ADRs](docs/adr)** — every load-bearing decision, with a comparison against the alternatives and the trigger that would reopen it. Start at [ADR-0000](docs/adr/0000-platform-foundations.md); the block map and the full index are in [`docs/adr/README.md`](docs/adr/README.md).
- **[ADR-0001](docs/adr/0001-documentation-and-output-conventions.md)** — the rules these documents are written to: density, banned constructs, section order, and the same rules for code comments.
- **Rules sections** — each ADR ends with flat, greppable normative statements, optimised for humans and LLMs to apply consistently.
- **Fitness functions** — rules are enforced by machines wherever possible: admission control, type checking, drift-checked codegen, and static analysis for architectural invariants. Every rule is annotated with how it is enforced ([ADR-0001](docs/adr/0001-documentation-and-output-conventions.md)), so a reader tells a hard invariant from an aspiration. Review is the fallback, not the mechanism.

---

## Prior art

This template borrows its framing rather than inventing it:

- **Richards & Ford, *Fundamentals of Software Architecture*** — architecture styles rated against characteristics as risk profiles. The source of the profile table above, and of the term *service-based architecture*.
- **Team Topologies** — cognitive load as the sizing unit; *thinnest viable platform*.
- **DORA / *Accelerate*** — capability models over maturity ladders.
- **Ford, Parsons & Kua, *Building Evolutionary Architectures*** — fitness functions.
- **Poppendieck, *Lean Software Development*** — the last responsible moment.
- **Feathers, *Working Effectively with Legacy Code*** — seams.
- **Fowler — *MonolithFirst*, *MicroservicePremium*** — the prerequisite bar for decomposition.
- **12-Factor**, **Choose Boring Technology**, **CNCF Platforms White Paper**.

---

## Using this repo

- **It is a template, not a product.** Fork it and own your copy. There is no support channel and no compatibility promise.
- **Generated projects** track upstream via Copier's 3-way merge.
- **Disagreement is expected.** The ADRs record reasoning precisely so you can overturn a decision on purpose rather than by accident. If a comparison table is wrong, that is the most useful issue you can open.
