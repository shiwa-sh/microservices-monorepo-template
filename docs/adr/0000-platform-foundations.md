# ADR-0000: Platform Foundations

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0001](0001-documentation-and-output-conventions.md)

## Purpose

This ADR is referenced by every other ADR. It pins down:

1. **The thesis** — the platform's position on three axes, and what follows from it.
2. **The principles** every later decision must honour, each anchored to an external standard or marked local.
3. **The ADR process** — how decisions are made, recorded, changed.
4. **Prior art** — the sources this document's framing rests on.
5. **Vocabulary** used consistently across the rest of the documents.

No technology is chosen here. The rest of the ADRs choose technology.

## Thesis

This is a **self-hosted product platform**: its parts ship, scale, and fail independently, the owning organisation controls every piece of its infrastructure, and a wrong answer costs money. Every reusable choice is pre-decided, so work starts at "build features" rather than "pick stacks."

We state no target service count. A service count is an **outcome**, not a target: a number encodes a set of pressures, and once written down the number hides them. What follows names the pressures directly.

### The three axes

Architecture is forced by three largely independent questions. Position on each determines the decisions below; product category ("fintech", "B2B SaaS", "marketplace") determines almost nothing, because two products in the same category routinely sit at opposite ends of every axis.

| Axis | The question | Range |
| --- | --- | --- |
| **A — Decomposition pressure** | Must the parts ship, scale, or fail *independently*? | monolith → modular monolith → few services → many |
| **B — Operational sovereignty** | Who is permitted to run it? | managed cloud → hybrid → fully self-hosted |
| **C — Correctness stakes** | What does a wrong answer cost? | cosmetic → transactional → financial / regulated |

**Our position: A high, B maximal, C high.** Axis B is the distinguishing choice: most platforms at this position on A and C assume managed services underneath. This one does not, and that single decision generates more of the machinery in this repo than A and C combined.

Later ADRs cite this position rather than restating it. It changes here, in one line, and re-opens every ADR that rests on it.

**Positions on these axes are not maturity levels.** A monolith is a correct *terminal* state for most systems, not a waypoint. A higher position is not better; it is a more expensive answer to a pressure a system may not have. Where this repo does use levels — the CNCF project tiers in principle 4 — they are *recorded evidence about a dependency*, never a target for ourselves.

### Axis A: what forces decomposition

Service count is derived, not chosen. Each force below independently justifies a boundary.

| # | Force | The test |
| --- | --- | --- |
| 1 | Independent deploy cadence | Two parts must ship without coordinating a release |
| 2 | Number of teams | Teams block each other. Conway's law — teams, not headcount, not features |
| 3 | Failure isolation | One part failing must not take another down, as a *stated requirement* |
| 4 | Resource heterogeneity | Genuinely different scaling profiles — CPU-bound vs IO-bound vs memory-bound |
| 5 | Technology heterogeneity | A part must run on another runtime, per an [ADR-0100](0100-language-and-runtime.md) escape hatch |
| 6 | Compliance boundary | Data residency or audit scope cheaper to enforce structurally than by policy |

**A new service names the force that justifies it**, and the absence of all six means do not split. Nothing here obliges a high service count: the same machinery serves three deployables, and several ADRs get cheaper at that size.

### Capacity is a separate question from need

The axes say what a system *should* be. They say nothing about whether it can be operated.

> **Need determines what you build. Capacity determines what you can run. The gap between them is the risk.**

Team size is therefore not a design driver — it is a budget. It does not tell a project to build thirty services; it tells the project what it can afford to operate. Stating a platform's target as a headcount conflates the two questions and produces a thesis whose halves never reconcile.

The capacity this position on axis B requires:

- **An always-on platform floor of a couple of dozen components** — gateway, GitOps, identity, authz, workflow, data, storage, observability, registry, forge, outbound mail. The floor is **fixed**: it does not shrink with fewer services. [`docs/operational-surface.md`](../operational-surface.md) is the inventory and the only place the components are counted.
- **2–3 engineers whose primary job is the platform**, not product engineers with a platform rotation. This is the most common way a self-hosted platform rots.

An important consequence: platform cost is fixed while application cost is variable, so **the fewer services run, the heavier the platform is proportionally.** At many services the ratio of application to platform workloads is comfortable; at a handful they are roughly equal. Fewer services therefore *strengthens* the case for a small, austere platform rather than relaxing it.

### Moving down axis B

Axis B is a genuine axis, not a virtue test. Sovereignty weaker than maximal is a legitimate position, recorded in an ADR of its own rather than drifted into. It is the right answer for a team that cannot staff the platform, and a better one than holding the self-hosted floor and hoping.

**Axis B is the axis this platform is least coupled to.** Moving down it swaps *operators*, not architecture. Unchanged by a managed swap: the ADR set, OpenAPI contracts and codegen, the service template, the Kustomize tree, policy-as-admission, repo layout, release and versioning, and the local dev loop.

Swap in this order, ranked by capacity returned per unit of sovereignty conceded rather than by ease:

| # | Self-hosted here | Managed equivalent | Why this rank |
| --- | --- | --- | --- |
| 1 | Outbound email (MTA) | Any transactional email provider | Deliverability is reputational, not technical — the one cost engineering effort cannot retire. Concede this first even at high sovereignty |
| 2 | Alert routing and on-call | A hosted paging service | No mature self-hosted escalation layer exists; the alternative is a rota and a phone |
| 3 | PostgreSQL (CNPG) | Managed Postgres | Highest operational risk per unit of engineering time. Backups, PITR, failover, and major upgrades all become someone else's rota |
| 4 | Object storage (Rook-Ceph) | Any S3-compatible service | Removes a distributed storage system and the separate-failure-domain backup problem in one move |
| 5 | Observability (the Grafana stack) | A hosted observability backend | A backend family to one vendor. OpenTelemetry instrumentation is unchanged, which is the point of [ADR-0500](0500-observability.md)'s OTel-first rule |
| 6 | Temporal | Temporal Cloud | Workflow code is identical; only the connection target changes |
| 7 | Forge and CI | A hosted forge | Cheap to move either way, because `mise run ci:*` keeps workflows thin |
| 8 | Image registry | A hosted registry | Verify Cosign policy and scanning parity before swapping |
| 9 | Identity (Ory) | A hosted IdP | Late, not first: [ADR-0304](0304-identity-and-authorization.md)'s headless requirement narrows the field, and identity data is the most painful to migrate twice |
| 10 | Kubernetes | Managed Kubernetes | Keep the API, drop the substrate. Removes [ADR-0200](0200-cluster-topology.md)'s node provisioning entirely |

A **hybrid** position is legitimate and common: self-host the orchestrator and the stateless platform, take managed Postgres, object storage, email, and paging. That removes most of the pager burden while keeping the parts whose sovereignty usually motivated the choice. Reversal is a migration per component in either direction, so the position is chosen deliberately.

A system that has taken most of that list is no longer at A-high/B-maximal, and the machinery here may exceed its need.

### What follows from the thesis

- **Per-service cost compounds.** A choice that adds RAM, CI time, or onboarding cost per service is multiplied by the service count. At many services this is existential; at few it is an efficiency — but it is never free.
- **One way to do things.** When a tool or pattern is picked, it is picked for everyone. Per-service deviation requires a new ADR.

The rest of what follows is stated as principles below, since each one also decides tools.

## Principles

The principles below decide everything else in this repo. Every later ADR is checked against them. If a decision violates a principle, the principle wins or the principle is changed *in this document* — never silently waived.

**The test for being on this list:** a principle earns its place only if it has **rejected something we wanted**. Anything that cannot name a casualty is a virtue statement, not a criterion, and does not belong here.

Each principle is **anchored** to an external standard, or explicitly marked **local** where none applies. The distinction is load-bearing: a borrowed criterion survives re-litigation; a house rule gets re-argued every year. A reader should be able to tell which is which without asking.

### Selection — how a tool gets in

**1. Configuration lives in the repository, not in the component.** Dashboards, alert rules, identity schemas, OAuth2 clients, authz models. If the source of truth is the running system's database and the UI is the editor, it is invisible to review, invisible to Argo, and unreproducible from git.

- *Anchor:* [OpenGitOps](https://opengitops.dev/) principles 1 (*Declarative*) and 2 (*Versioned and Immutable*), v1.0.0, CNCF GitOps Working Group — **extended one level out**. OpenGitOps governs the system state a GitOps agent reconciles; we apply the same requirement to each platform component's own configuration. The extension is ours; the principle is not.
- *Rejected:* SigNoz, OpenObserve, Coroot ([ADR-0501](0501-operator-uis-and-dashboards.md)), Zitadel ([ADR-0304](0304-identity-and-authorization.md)).
- *What it costs:* an observability stack of several components instead of one product ([ADR-0500](0500-observability.md)). This principle is expensive and is applied anyway. That is what makes it a principle.

**2. Thinnest viable platform — the always-on floor is the budget.** A simpler tool covering 90% of the need beats a richer tool that needs a dedicated operator. Components are tiered Core / Scale / Opt-in in [`docs/operational-surface.md`](../operational-surface.md); a component joins Core only when no existing Core component covers its concern.

- *Anchor:* Team Topologies, [*Thinnest Viable Platform*](https://teamtopologies.com/key-concepts-content/what-is-a-thinnest-viable-platform-tvp) — the smallest set of APIs, docs, and tools that still accelerates the teams building on it.
- *Rejected:* GitLab CE, Coroot, every second CI system.

**3. Operational sovereignty — we run it ourselves.** Not "self-host by default" — this is **axis B at maximum**, a hard constraint rather than a preference with an exit. Every component runs on infrastructure the owning organisation controls, and managed services are not adopted to reclaim operational budget.

- *Anchor:* axis B, above. The position is ours; the axis is not.
- *Rejected:* every managed service, including where one would plainly be cheaper.

*Why there is no soft exit.* The tempting version of this principle moves components to managed when the platform crowds out feature work. That inverts the causality the thesis states: **capacity is the thing to change, not the constraint.** Where sovereignty is genuinely binding — data residency, regulatory control, cost predictability, or a refusal to depend on third-party operators — a capacity shortfall is resolved by hiring. Where it is not binding, the position on axis B is different and is recorded in its own ADR, not treated as an escape hatch inside this one.

*Consequence, stated plainly.* Axis B at maximum removes a set of services that are otherwise invisible defaults: transactional email delivery ([ADR-0307](0307-outbound-email.md)), alert routing ([ADR-0502](0502-alerting-and-on-call.md)), source control and CI ([ADR-0102](0102-source-control-and-ci.md)), image registry ([ADR-0105](0105-image-registry.md)), object storage ([ADR-0200](0200-cluster-topology.md)), and single sign-on across platform consoles ([ADR-0304](0304-identity-and-authorization.md)). Each is a first-class decision here rather than a signup. Some — outbound email deliverability in particular — carry costs that engineering effort cannot fully retire.

*Where axis B is not maximal.* Maximum is a position, not a claim of zero external dependencies. Six remain, each recorded rather than discovered later.

| Dependency | Why it stays | Blast radius if it fails |
| --- | --- | --- |
| Compute and network provider | [ADR-0200](0200-cluster-topology.md) runs on plain instances, which someone provisions. Sovereignty is over the *stack*, not the metal | total for that environment. Mitigated by the provider-neutral bootstrap, which is what makes the substrate replaceable |
| Sigstore public Fulcio and Rekor | [ADR-0104](0104-supply-chain-security.md) trades key custody for a hosted trust root, deliberately | signing blocks; running does not. A private Sigstore deployment is the exit, at the cost the keyless choice avoided |
| Public ACME certificate authority | [ADR-0200](0200-cluster-topology.md) issues wildcards through cert-manager against a public CA | renewals fail, with the certificate lifetime as the buffer. A private CA is the exit, at the cost of distributing a trust anchor |
| DNS registrar and authoritative DNS | the domain is delegated, and DNS-01 needs a provider API | edge unreachable, and issuance blocked. This is the least substitutable item on the list |
| Upstream package and image sources | Go modules, npm, Helm charts, and base images are fetched from where their publishers put them | builds fail; running does not. Digest pins ([ADR-0104](0104-supply-chain-security.md)) make what already runs immune |
| Escalation and paging | no mature self-hosted layer exists ([ADR-0502](0502-alerting-and-on-call.md)) | deferred rather than depended on. Nothing pages today |

None of these is a soft exit from principle 3. Each is a dependency at a layer the platform does not claim to own, or a recorded concession with a named cost. A dependency that is not on this list and not decided in an ADR is a defect.

**4. Spend novelty by exit cost, not by taste.** Novelty is a fixed budget, and *where* it is spent matters as much as how much. Be adventurous where abandonment costs days — CI, observability query layers, registries, policy engines. Be conservative where it costs months and customer data — object storage, databases, the message bus. "Actively maintained" is adequate assurance for the first class and inadequate for the second.

- *Anchor:* McKinley, [*Choose Boring Technology*](https://mcfunley.com/choose-boring-technology) — innovation tokens as a budget rather than a preference. Note the budget is *about three*; we are over it, deliberately and with sequencing as the mitigation.
- *Rejected:* Garage and SeaweedFS (too young for the data plane), Encore (a data-plane-shaped lock-in wearing control-plane clothing).
- *Accepted on the same rule:* Talos, Kyverno, mirrord, Microcks — all young, all cheap to leave.
- Licence, governing body, and project maturity (CNCF tier, Thoughtworks Radar ring) are **recorded** per tool but do **not** veto. Provenance is evidence about exit cost, not a rule of its own.

**5. One primitive per concern.** No parallel mechanisms for the same problem: one workflow engine ([Temporal](0302-temporal.md)), one queue model, one deploy mechanism, one manifest tree. Durable, multi-step, or cross-service async is a Temporal workflow; a trivial best-effort job may use the sanctioned transactional-outbox path — the same concern served by one lighter primitive, not a competing engine.

- *Anchor:* **local** — falls out of principle 2. Every parallel mechanism is a second runbook.
- *Rejected:* Woodpecker, Tekton, Argo Workflows, Jenkins (all viable; all a second system beside the forge).

**6. Two languages, and no ambient runtime.** Go and TypeScript. Escape hatches (Rust, Python) require their own ADR with a measured need. No tool may assume a runtime that is not pinned in `.mise.toml`.

- *Anchor:* [12-Factor II](https://12factor.net/dependencies) — *"A twelve-factor app never relies on implicit existence of system-wide packages"* — for the ambient-runtime half. The two-language cap itself is **local**.
- *Rejected:* Semgrep and sqlfluff (both need ambient Python), which is why static analysis is `ast-grep` and SQL lint is `sqruff`.

**7. No adoption without a recorded comparison.** A tool with no recorded comparison against its alternatives is an assumption, not a decision.

- *Anchor:* **local** — principle 4 applied to the record. Depth is set by exit cost: paragraphs where abandonment costs months and customer data, a line where it costs days.
- *Rejected:* nothing directly — this is the process gate that makes principles 1–6 enforceable rather than aspirational.

### Construction — how the system is built once a tool is in

**8. Local–prod parity at the manifest and API layer.** Topology may differ; charts, code, and commands do not.

- *Anchor:* [12-Factor X](https://12factor.net/dev-prod-parity) — *"resists the urge to use different backing services between development and production."*
- *Rejected:* Docker Compose for local development — a second definition of the system, whose drift from the real manifests fails at runtime rather than build time.

**9. Generated code is committed and drift-checked in CI.** Drift is caught in review, not at runtime.

- *Anchor:* **local** — principle 1 applied to generated artifacts.
- *Rejected:* generation at build, deploy, or run time.

**10. Service boundaries are HTTP/OpenAPI.** Services compose through generated clients — never shared code, never a shared database, never a direct workflow call into another service's namespace.

- *Anchor:* Fowler, [*IntegrationDatabase*](https://martinfowler.com/bliki/IntegrationDatabase.html) — a shared database *"becomes a point of coupling between the applications that access it… a deep coupling that significantly increases the risk involved in changing those applications."*
- *Rejected:* shared libraries and shared schemas as integration mechanisms.

## ADR process

### How an ADR must be written

Two rules that are easily mistaken for principles. They are not selection criteria — they govern *how a decision is recorded*, and so they live here.

**Decisions are unambiguous.** When an ADR says "do X," there is no "or Y in some cases." Conditional behaviour is expressed as a **measurable trigger**, not a judgement call. An ADR that requires taste to apply has not finished deciding.

**No "temporary."** Anything described as temporary becomes permanent. Either commit to a solution, or defer it behind a hard trigger — and a deferral is only complete when three things are written down:

| Field | Requirement |
| --- | --- |
| **Trigger** | An *observable* condition. "When we grow" is not a trigger; "when Prometheus exceeds its retention window" is |
| **Seam** | Whether the slot already exists for the thing to drop into |
| **Cost if adopted late** | What the delay buys, and what it risks |

A deferral **with a seam** is additive — adopting it later changes configuration, not structure. A deferral **without a seam** is a *bet*, not a deferral, and must be labelled as one in the owning ADR. The two look identical in a backlog and are different in a review. [`docs/operational-surface.md`](../operational-surface.md) holds the Core / Scale / Opt-in tiers this applies to, and the current list of what is deferred.

### When to write an ADR

Write an ADR when a decision will be **load-bearing for more than one service** or **hard to reverse**. Examples:

- Choosing or replacing a platform-wide tool (database, gateway, IdP, observability backend).
- Defining a cross-service contract (auth headers, error format, workflow handle shape).
- Sanctioning a per-service exception (a Rust service, a non-default storage class, a long-running workflow).

Single-service implementation details do **not** get an ADR. They live in the service's README.

### Statuses

- **Proposed** — open for review. Linked in a PR; not yet binding.
- **Accepted** — merged. Binding on all services.
- **Superseded by ADR-XXXX** — replaced by a newer decision, kept for history.
- **Amended by ADR-XXXX** — modified in part. The amendment cites which sections it changes.

There is no `Rejected` or `Draft` status. Rejected proposals are closed PRs, not committed files.

**Scope:** the last two statuses belong to a project generated from this template, which is a living system with real history. **This template's own set carries no history** — every ADR here reads `Accepted`, carries the date the set was written, and one that must change is rewritten in place ([ADR-0001](0001-documentation-and-output-conventions.md)).

### Structure, numbering, and prose

[ADR-0001](0001-documentation-and-output-conventions.md) owns all three: the section order every ADR follows, the blocks-of-a-hundred numbering, and the density rules the prose is held to. One rule belongs here instead, because it is a selection criterion rather than a style: **the comparison table in *Considered options* is mandatory** (principle 7), at a depth set by that tool's exit cost. A long deep-dive belongs in the PR discussion, not the ADR.

### Who decides

The platform team. One reviewer with platform-team approval is sufficient for an ADR to merge. Disagreement is resolved by a 24-hour async comment window; unresolved disagreements escalate to a synchronous decision call.

## Prior art

This ADR borrows its framing rather than inventing it. Where a decision here has an external anchor, it is cited at the point of use; these are the sources the structure itself rests on.

| Source | What this document takes from it |
| --- | --- |
| Richards & Ford, *Fundamentals of Software Architecture* | Architecture styles rated against characteristics as **risk profiles**, not levels. The term *service-based architecture* |
| DORA / *Accelerate* | Capability models over maturity ladders |
| Team Topologies (Skelton & Pais) | Cognitive load as the sizing unit; **thinnest viable platform** (principle 2) |
| [OpenGitOps](https://opengitops.dev/) v1.0.0, CNCF | Declarative and versioned configuration (principle 1) |
| [12-Factor](https://12factor.net/) | Explicit dependencies (principle 6); dev/prod parity (principle 8) |
| McKinley, [*Choose Boring Technology*](https://mcfunley.com/choose-boring-technology) | Innovation tokens as a budget (principle 4) |
| Fowler, [*IntegrationDatabase*](https://martinfowler.com/bliki/IntegrationDatabase.html), *MonolithFirst*, *MicroservicePremium* | Service boundaries (principle 10); the prerequisite bar for decomposition |
| Ford, Parsons & Kua, *Building Evolutionary Architectures* | **Fitness functions** — the Rules sections below are meant to be executable, not advisory |
| Poppendieck, *Lean Software Development* | The **last responsible moment** — deferral with a written trigger |
| Feathers, *Working Effectively with Legacy Code* | **Seams** |
| Conway (1968) | Boundaries track teams, not features (axis A, force 2) |

## Vocabulary

Used consistently across all ADRs. If a term is ambiguous in a later ADR, this glossary wins.

- **Axis position** — this platform's load-bearing assumption, set in *Thesis* above: **A high, B maximal, C high**. An ADR cites the axis position rather than restating a service count or a headcount, so it changes in that one place.
- **Seam** — a pre-built slot a deferred capability drops into without restructuring. A deferral with a seam is additive; one without is a bet.
- **Anchor** — the external standard a principle rests on. A principle without one is marked **local**.
- **Service** — a deployable unit under `services/<name>/` owning a slice of business state and an OpenAPI surface. May include HTTP server, Temporal worker, and migrations.
- **Frontend app** — the single Next.js application under `apps/frontend/`. Route groups separate audiences; the deploy unit is one app.
- **Platform component** — infrastructure software the services depend on (Postgres, Temporal server, gateway, IdP, observability stack). Lives under `infra/helm/platform/`.
- **Library** — shared code under `libs/go/<name>/` or `libs/ts/<name>/`. Has no business state.
- **Generated client** — code under `libs/{go,ts}/sdks/<service>/` produced from a service's OpenAPI spec. Committed to the repo.
- **Workflow / Activity** — Temporal terms. See ADR-0302.
- **Authz-relevant mutation** — a state change that affects who can see or modify a resource. See ADR-0304.
- **Affected** — in CI, the set of services and apps a PR's diff influences. See ADR-0101.
- **Environment** — one of `dev`, `staging`, `prod`. Each is a single cluster on day one. See ADR-0200.

## Consequences

### Positive

- Newcomers (humans or LLMs) read one short document and have the platform's worldview.
- Later ADRs do not relitigate principles; they cite them.
- Coherence is a checkable property: an ADR either follows the principles or amends them.

### Negative / Risks

- **The axis positions are the load-bearing assumption.** A system that sits materially lower on axis A or B needs several later ADRs re-evaluated. Acknowledged, not mitigated — *Moving down axis B* above exists so that re-evaluation is guided rather than improvised.
- **The list is longer than most people hold in their head.** The mitigation is that most are anchored to standards a reader may already know, and that every one names what it rejected — a criterion with a casualty is easier to recall than an abstraction.
- Strong opinions reduce flexibility. A team that needs to deviate justifies it in a new ADR.

### Follow-ups

None — this ADR is the root.

## Rules

### Process

- An ADR exists for every decision that binds more than one service or is hard to reverse. `(review-only)`
- An ADR follows the structure, numbering, and prose rules of [ADR-0001](0001-documentation-and-output-conventions.md). `(review-only)`
- **No tool is adopted without a recorded comparison against its alternatives** (principle 7). The comparison is a table with prose cells, not a scoring grid, at a depth set by that tool's exit cost. `(review-only)`
- An accepted ADR is binding on every service. Per-service deviation requires a new ADR. `(review-only)`
- A decision is unambiguous: no "or Y in some cases" without a measurable trigger. `(review-only)`
- Nothing is "temporary." Either commit, or defer behind a hard trigger — and record the trigger, whether a **seam** exists, and the cost of adopting late. A deferral without a seam is a **bet** and is labelled one. `(review-only)`

### Selection

- Configuration lives in this repo, not in the component. UI-only state is not allowed for anything reconciled into a cluster, and not for a component's own configuration either. `(review-only)`
- Platform components are tiered Core / Scale / Opt-in in [`docs/operational-surface.md`](../operational-surface.md). A component joins Core only when no existing Core component covers its concern; a Scale variant replaces its Core counterpart only on the measured trigger documented there. `(review-only)`
- Every component runs on infrastructure we control. Managed services are not adopted to reclaim operational budget. `(review-only)`
- Novelty is spent by exit cost: freely where abandonment costs days, conservatively where it costs months and customer data. `(review-only)`
- One primitive per concern. Parallel mechanisms for the same problem require an ADR that retires the incumbent. `(review-only)`
- Go and TypeScript only. No tool may assume a runtime that is not pinned in `.mise.toml`. `(CI: lint:node-scope)`
- Licence, governing body, and project maturity are **recorded** for every component and **do not veto** a choice. `(review-only)`

### Construction

- Local and production differ in topology only. Charts, code, and commands do not diverge. `(review-only)`
- Generated code is committed and drift-checked in CI. `(CI: ci-drift)`
- Service-to-service communication is HTTP via generated OpenAPI clients. Shared databases, shared code, and cross-service workflow calls are forbidden. `(CI: ci-lint)`
- Local development uses the same Helm charts, container images, and commands as production. Topology may differ; interface may not. `(review-only)`
