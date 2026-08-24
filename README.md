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

| Profile | You are here if… | Style | Deployables | Platform floor | What platform work looks like |
| --- | --- | --- | --- | --- | --- |
| `modular-monolith` | No decomposition force applies (see below) | Modular monolith | 1 | whatever the provider runs for you | nobody's job — the provider operates it |
| `service-based` | Teams block each other on deploys | Service-based | 3–8 | a handful, on managed foundations | part of a backend role |
| `microservices` | Coordination cost dominates engineering cost | Microservices | 15+ | substantial, still on managed foundations | a standing responsibility someone owns by name |
| **`sovereign`** ← **this repo** | Self-hosting is *binding*, not preferred | Microservices | 15+ | **everything, and it is fixed regardless of service count** | **a primary responsibility, and never resting on one person** |

Only the last two rows differ on sovereignty rather than on architecture, and that one difference generates most of this repo. The positions are set in [ADR-0000](docs/adr/0000-platform-foundations.md); this repo's own component inventory is [`docs/operational-surface.md`](docs/operational-surface.md), which is the only place they are counted.

> **These are not maturity levels.** A modular monolith is a correct *terminal* state for most systems, not a waypoint on the road to microservices. Higher rows are not better — they are more expensive answers to pressures you may not have. This follows Richards & Ford's treatment of architecture styles as **risk profiles rated against characteristics**, and deliberately rejects the numbered-ladder genre (CMMI, and the maturity models that followed it) — a framing DORA/*Accelerate* also argues against.

**This repo implements `sovereign`.** The other three rows are positions to find yourself in, not presets to generate — a project at one of them is better served by a smaller template than by this one with most of it deleted.

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
- **Platform work as a primary responsibility, not a rotation** — a rotation optimises for the incident in front of it, and this floor's real cost is the upgrades and drills nothing forces you to do this week. This is the most common way a self-hosted platform rots
- **No Core component whose only competent operator is one person** — the floor stays operable through a departure and an absence at the same time
- Component count is **fixed**. It does not shrink with fewer services

**No headcount is stated, and none should be inferred.** What it takes depends on your coverage hours, your existing operational skill, and your tolerance for detection latency. `docs/operational-surface.md` carries the recurring obligation and failure-response requirement of every component — sum that against your own facts and you get your number, not ours.

**If the number comes out higher than you have**, [`docs/adoption-path.md`](docs/adoption-path.md) is the ranked list of what to give up and in what order: deferrals first, then managed swaps, and the point past which you are running a different platform.

**Building it with an LLM changes the authoring cost and none of the rest.** Generating a service, a chart, or a migration is now cheap, which moves the bottleneck rather than removing it: operational surface is unchanged, and coherent review — knowing that a generated change is consistent with the twenty decisions it touches — gets harder as the volume of generated change goes up. That is an argument about capacity, not about need, and it belongs on this side of the line. The ADRs and the Rules sections exist partly for this: they are the constraint an agent is held to when nobody has time to re-derive the reasoning.

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
| Escalation and paging | An on-call rota exists | ⚠️ **no credible self-hosted escalation layer exists** — routing is Alertmanager, paging is a hosted service ([0502](docs/adr/0502-alerting-and-on-call.md)) | Opt-in |

Deferring on purpose, with the trigger written down, is the **last responsible moment** applied to architecture — not the same thing as not having decided.

---

## When not to use this

- **No decomposition force applies** → `modular-monolith`. This is the common case.
- **Platform work would be a rotation rather than someone's primary job** → the floor is heavier than its component count suggests. Read [`docs/adoption-path.md`](docs/adoption-path.md) before adopting, and move down axis B deliberately rather than discovering the gap in an incident.
- **Managed services are acceptable to you** → much of this repo solves a problem you do not have.
- **Correctness stakes are low** → the typing, contract, and policy discipline here is priced for money-handling systems and will read as friction.

---

## Lower on the sovereignty axis

Axis B is an axis, not a virtue test. **A smaller team should sit lower on it deliberately** — that is a better answer than adopting the self-hosted floor and hoping.

Axis B is also the axis this template is **least coupled to**: moving down it swaps *operators*, not architecture. Unchanged by a managed swap — the ADRs, OpenAPI contracts and codegen, the service template, the Helm and GitOps trees, policy-as-admission, repo layout, release flow, and the local dev loop.

[`docs/adoption-path.md`](docs/adoption-path.md) is the ranked order to swap in, and the canonical one. It carries three bands — deferrals that cost only the wait, managed swaps that cost sovereignty, and capability concessions that are bets — with a restore trigger and a cost-to-reverse per row, and the line past which a project is running a different platform.

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
| 2 | The always-on floor is the budget, measured in concerns | [Team Topologies](https://teamtopologies.com/key-concepts-content/what-is-a-thinnest-viable-platform-tvp) — cognitive load as the unit, applied to operators rather than users | GitLab CE, Coroot, a second CI system |
| 3 | Operational sovereignty — run it yourself | axis B, above | every managed service |
| 4 | Spend novelty by exit cost, not by taste | [Innovation tokens](https://mcfunley.com/choose-boring-technology) | Garage, Encore |
| 5 | One primitive per concern — no parallel mechanisms for one problem | **local** — falls out of principle 2 | Woodpecker, Tekton, Argo Workflows |
| 6 | Two languages, and no ambient runtime | [12-Factor II](https://12factor.net/dependencies) — *"never relies on implicit existence of system-wide packages"*; the two-language cap itself is **local** | Semgrep, sqlfluff (both Python) |
| 7 | No adoption without a recorded comparison | **local** — principle 4 applied to the record ([0002](docs/adr/0002-tool-adoption.md) sets the depth) | *the process gate for all of the above* |

**Construction — how the system is built once a tool is in:**

| # | Principle | Anchored to | It rejected |
| --- | --- | --- | --- |
| 8 | Local–prod parity at the manifest and API layer | [12-Factor X](https://12factor.net/dev-prod-parity) — *"resists the urge to use different backing services between development and production"* | Docker Compose — a second definition of the system |
| 9 | Generated code is committed and drift-checked in CI | **local** — principle 1 applied to generated artifacts | codegen at runtime or at deploy |
| 10 | Service boundaries are HTTP/OpenAPI — never shared code, never a shared database | Fowler, [*IntegrationDatabase*](https://martinfowler.com/bliki/IntegrationDatabase.html) — *"integration databases should be avoided"* | shared libraries and shared schemas as coupling |

Principle 4 is worth reading twice, because it is the one most often misread as conservatism: **be adventurous where abandonment costs days** (CI, query layers, registries, policy engines) **and conservative where it costs months and customer data** (storage, databases, the message bus). Talos, Kyverno, and zot all sit on the cheap-exit side of that line; SeaweedFS and CNPG sit on the expensive side and are chosen accordingly.

---

## Stack at a glance

Every row is a decision recorded in an ADR, with a comparison against the alternatives: options considered, what each was judged on, why the alternatives lost, and what would reopen it. A tool with no recorded comparison is an assumption, not a decision.

Every tool here also carries a row in [`docs/tool-register.md`](docs/tool-register.md), with its exit-cost tier, licence, governing body, and the alternatives it was recorded against.

| Concern | Current decision | ADR |
| --- | --- | --- |
| Backend language | Go | [0100](docs/adr/0100-language-and-runtime.md) |
| Frontend | Next.js + TypeScript on Bun, one app with route groups | [0100](docs/adr/0100-language-and-runtime.md), [0400](docs/adr/0400-frontend.md) |
| Design system | Untitled UI React, vendored as source, on Tailwind | [0400](docs/adr/0400-frontend.md) |
| Task runner | `mise` | [0101](docs/adr/0101-monorepo.md) |
| Machines | Talos Linux, configured by machine config | [0200](docs/adr/0200-cluster-topology.md) |
| Cloud resources | Terraform, per project, skipped where infrastructure is pre-provided | [0200](docs/adr/0200-cluster-topology.md) |
| Cluster | upstream Kubernetes on etcd in every tier, on kind for both local tiers | [0200](docs/adr/0200-cluster-topology.md), [0600](docs/adr/0600-local-development-loop.md) |
| Deploy | Argo CD, the only mechanism; one shared Helm chart, per-env values | [0201](docs/adr/0201-gitops.md) |
| Network / policy | Cilium + Hubble, WireGuard, default-deny | [0206](docs/adr/0206-cluster-networking.md) |
| Policy enforcement | Kyverno at admission, PSA `restricted`, CiliumNetworkPolicy, CI lints | [0203](docs/adr/0203-policy-enforcement.md) |
| Resource governance | LimitRange, ResourceQuota, PriorityClass, PDB | [0204](docs/adr/0204-resource-management.md) |
| Edge | Traefik + Ory Oathkeeper + cert-manager | [0305](docs/adr/0305-edge-auth-and-traffic-policy.md) |
| Identity / authz | Ory Kratos + OpenFGA; Hydra when a public API exists | [0304](docs/adr/0304-identity-and-authorization.md) |
| Trust tiers | product apex, `*.ops.<host>` for operator tooling | [0306](docs/adr/0306-trust-tiers-and-urls.md) |
| Secrets | SOPS + age, decrypted in-cluster by an operator | [0202](docs/adr/0202-secrets.md) |
| Database | PostgreSQL via CNPG; sqlc, dbmate, sqruff | [0300](docs/adr/0300-data.md) |
| Object storage | SeaweedFS everywhere, run outside the cluster in prod with Object Lock | [0207](docs/adr/0207-cluster-storage.md), [0205](docs/adr/0205-environment-parity.md) |
| Workflows | self-hosted Temporal, versioned workers | [0302](docs/adr/0302-temporal.md) |
| API contract | hand-written OpenAPI 3.1; ogen, openapi-typescript, vacuum, oasdiff | [0303](docs/adr/0303-api-contracts-and-lifecycle.md) |
| API mocking | Prism, vendored, from the committed projection | [0600](docs/adr/0600-local-development-loop.md) |
| Observability | OpenTelemetry → Grafana, Loki, Tempo, Prometheus, Pyroscope | [0500](docs/adr/0500-observability.md) |
| Operator UIs | Grafana funnel, Hubble UI, Headlamp | [0501](docs/adr/0501-operator-uis-and-dashboards.md) |
| Alerting | Prometheus rules + Alertmanager; escalation is conceded to a hosted service | [0502](docs/adr/0502-alerting-and-on-call.md) |
| Error tracking | OTel exceptions with a computed `error.fingerprint`; no separate product | [0503](docs/adr/0503-error-tracking.md) |
| Source control + CI | Forgejo + Forgejo Actions, entered through `mise run ci:*`; a provider-hosted forge is the bootstrap path | [0102](docs/adr/0102-source-control-and-ci.md) |
| Supply chain | cosign with a key pair held in SOPS, syft SBOM, SLSA provenance, Trivy as a merge gate | [0104](docs/adr/0104-supply-chain-security.md) |
| Registry | zot, one instance per environment, backed by object storage | [0105](docs/adr/0105-image-registry.md) |
| Testing | Playwright, k6, `go test`, `bun test` | [0601](docs/adr/0601-testing-strategy.md) |
| Internal admin | Lowdefy over the service APIs; pgweb read-only | [0401](docs/adr/0401-internal-admin.md) |
| Analytics | Faro events into a first-party service | [0700](docs/adr/0700-analytics.md) |
| Dependency updates | Renovate, grouped and digest-pinning | [0106](docs/adr/0106-dependency-updates.md) |
| Template propagation | Copier, with upstream tracked by 3-way merge | [0106](docs/adr/0106-dependency-updates.md) |

---

## Getting started

```sh
# One-time
curl https://mise.run | sh
mise install                    # pinned toolchain from .mise.toml
mise run setup                  # git hooks
mise run secrets:age            # generate your age key (ADR-0202)

# Local cluster — the same charts as production, on two tiers
mise run cluster:up             # the floor: edge, identity, Postgres, mocks
mise run cluster:up -- full     # the whole platform, ArgoCD-driven, incl. observability

# Inner loop on one service — native process, no image build
cd services/catalog
mise run server
```

Full walkthrough: [`docs/dev-loop.md`](docs/dev-loop.md).

---

## Layout

```text
services/<name>/     — Go service: OpenAPI contract, server, worker, sqlc, migrations
apps/frontend/       — one Next.js app (route groups: landing|panel|devportal)
apps/admin/          — Lowdefy YAML for internal admin
libs/go/<name>/      — shared Go packages (observability, middleware, errors)
libs/{go,ts}/sdks/   — generated OpenAPI clients (committed, drift-checked in CI)
infra/helm/          — the shared service chart, and platform component charts
infra/gitops/        — Argo CD Applications and per-environment values
infra/talos/         — machine configs
infra/terraform/     — provisioning, where the project owns its infrastructure
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
- **Team Topologies** — cognitive load as the sizing unit, applied here to the people running the platform rather than the people building on it.
- **DORA / *Accelerate*** — capability models over maturity ladders.
- **Ford, Parsons & Kua, *Building Evolutionary Architectures*** — fitness functions.
- **Poppendieck, *Lean Software Development*** — the last responsible moment.
- **Feathers, *Working Effectively with Legacy Code*** — seams.
- **Fowler — *MonolithFirst*, *MicroservicePremium*** — the prerequisite bar for decomposition.
- **12-Factor**, **Choose Boring Technology**, **CNCF Platforms White Paper**.

### Where the published platform handbooks are stronger

The genre this repo is closest to is the internal engineering handbook — Netflix's [paved road](https://netflixtechblog.com/full-cycle-developers-at-netflix-a08c31f83249), [Monzo's](https://monzo.com/blog/2016/09/19/building-a-modern-bank-backend) account of running a bank on a large service estate, and [Shopify's](https://shopify.engineering/deconstructing-monolith-designing-software-maximizes-developer-productivity) writing on decomposition. The term *paved road* at the top of this file is Netflix's.

**They are better than this on the one thing a template cannot fake: operational narrative.** Each of those is written by a team describing a platform it has run at scale for years, so the reasoning is backed by outages that happened, migrations that went badly, and decisions that were reversed. That evidence is the most valuable part of the genre, and none of it is here.

What this offers instead is the falsifiable half: every decision states what it was compared against, what would reopen it, and how it is enforced — and [`docs/reference/`](docs/reference/) composes the decisions into the numbers they add up to, including the unflattering ones. Read the handbooks for what happens when a platform meets reality. Read this for the record of what was chosen and why, in a form you can argue with before you own the consequences.

---

## Using this repo

- **It is a template, not a product.** Fork it and own your copy. There is no support channel and no compatibility promise.
- **Generated projects** track upstream via Copier's 3-way merge.
- **Disagreement is expected.** The ADRs record reasoning precisely so you can overturn a decision on purpose rather than by accident. If a comparison table is wrong, that is the most useful issue you can open.
