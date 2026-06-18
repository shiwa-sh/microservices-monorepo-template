# ADR-0000: Platform Foundations

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Supersedes:** —

## Purpose

This ADR exists to be referenced by every other ADR. It pins down:

1. **The thesis** the platform optimises for.
2. **The principles** every later decision must honour.
3. **The ADR process** — how decisions are made, recorded, changed.
4. **Vocabulary** used consistently across the rest of the documents.

No technology is chosen here. The rest of the ADRs choose technology.

## Thesis

We build a **template** for microservice platforms targeting roughly **100 services** owned by a **small team (3–8 engineers)**, running on **self-hosted Kubernetes**. The template is opinionated: every reusable piece is pre-decided so that a new project starts at "build features" rather than "pick stacks."

This shapes every later trade-off:

- **Per-service cost dominates.** A choice that adds 100 MB of RAM, 30 s of CI, or a half-day of onboarding per service pays 100× the cost.
- **Operational surface dominates over feature breadth.** A simpler tool that covers 90% of needs beats a richer tool that needs a dedicated operator.
- **One way to do things.** When a tool or pattern is picked, it is picked for everyone. Per-service deviation requires a new ADR.

## Principles

Every later ADR is checked against these. If a decision violates a principle, the principle wins or the principle is changed in this document — never silently waived.

1. **Single primary language, single primary stack.** Polyglot is allowed only via explicitly sanctioned escape hatches with per-service justification.
2. **Self-host by default.** Managed services are not assumed. Cost predictability and data control win over operational convenience at the target scale. *Soft exit:* this is the principle most sensitive to team size, not service count — the self-host cost-benefit holds while the team can absorb the operational surface of the platform components. If a project adopts this template with a materially smaller team, or operating the platform stack consistently crowds out feature work, reconsider per-component (managed K8s/Postgres/Temporal/auth/observability) in a new ADR. This is a judgement call, not a measured trigger; the default stays self-host.
3. **Configuration as files in this repo.** GitOps-able, code-reviewed, testable in CI. No clicking in UIs to persist state.
4. **Local–prod parity at the manifest and API layer.** Topology may differ; charts, code, and commands do not.
5. **Generated code is committed.** Drift is caught in CI, not at runtime.
6. **One reliability primitive per concern.** No parallel mechanisms for the same problem (one workflow engine, one job runner, one queue model).
7. **Service boundaries are HTTP/OpenAPI.** Services compose through generated clients, not through shared code, shared databases, or direct workflow calls.
8. **Decisions are unambiguous.** When an ADR says "do X," there is no "or Y in some cases." Conditional behaviour is expressed as measurable triggers, not judgement calls.
9. **No "temporary."** Anything described as temporary becomes permanent. Either commit to a solution or defer the decision behind a hard trigger.

## ADR process

### When to write an ADR

Write an ADR when a decision will be **load-bearing for more than one service** or **hard to reverse**. Examples:

- Choosing or replacing a platform-wide tool (database, gateway, IdP, observability backend).
- Defining a cross-service contract (auth headers, error format, workflow handle shape).
- Sanctioning a per-service exception (a Rust service, a non-default storage class, a long-running workflow).

Single-service implementation details do **not** get an ADR. They live in the service's README.

### Statuses

- **Proposed** — open for review. Linked in a PR; not yet binding.
- **Accepted** — merged. Binding on all services.
- **Superseded by ADR-XXXX** — replaced by a newer decision. Kept in the repo for history; `Supersedes` is set in the replacement.
- **Amended by ADR-XXXX** — modified in part. The amendment ADR cites which sections it changes.

There is no `Rejected` or `Draft` status. Rejected proposals are closed PRs, not committed files.

### Structure

Every ADR uses these sections in this order:

1. **Header** (status, date, deciders, supersedes/related).
2. **Context** — the problem and the constraints inherited from earlier ADRs.
3. **Decision drivers** — what we are optimising for, in priority order.
4. **Considered options** — each rejected option in 2–4 lines. Long deep-dives belong in the PR discussion, not the ADR.
5. **Decision** — declarative. "We use X. We do not use Y."
6. **Consequences** — positive, negative/risks, follow-ups.
7. **Rules** — a flat, greppable bullet list of normative statements derived from the decision. Optimised for humans and LLMs to apply consistently.

### Changing an accepted ADR

- Small clarification: PR against the ADR file. The `Date` is updated; an `Amended` line is added if behaviour changed.
- Material change: a new ADR is written that supersedes the old one. The old file stays in place with `Status: Superseded by ADR-XXXX`.

### Who decides

The platform team. One reviewer with platform-team approval is sufficient for an ADR to merge. Disagreement is resolved by a 24-hour async comment window; unresolved disagreements escalate to a synchronous decision call.

## Vocabulary

Used consistently across all ADRs. If a term is ambiguous in a later ADR, this glossary wins.

- **Target scale** — the platform's load-bearing assumption, defined once here: **~100 services, a small team (3–8 engineers), self-hosted Kubernetes.** Later ADRs cite "the target scale" rather than restating these numbers; if the assumption ever changes, it changes in this one place. Domain-specific quantifications of it (e.g. "100+ sidecars on the hot path") stay in the ADR that makes the argument.
- **Service** — a deployable unit under `services/<name>/` owning a slice of business state and an OpenAPI surface. May include HTTP server, Temporal worker, and migrations.
- **Frontend app** — the single Next.js application under `apps/frontend/`. Route groups separate audiences; the deploy unit is one app.
- **Platform component** — infrastructure software the services depend on (Postgres, Temporal server, gateway, IdP, observability stack). Lives under `infra/helm/platform/`.
- **Library** — shared code under `libs/go/<name>/` or `libs/ts/<name>/`. Has no business state.
- **Generated client** — code under `libs/{go,ts}/sdks/<service>/` produced from a service's OpenAPI spec. Committed to the repo.
- **Workflow / Activity** — Temporal terms. See ADR-0006.
- **Authz-relevant mutation** — a state change that affects who can see or modify a resource. See ADR-0010.
- **Affected** — in CI, the set of services and apps a PR's diff influences. See ADR-0002.
- **Environment** — one of `dev`, `staging`, `prod`. Each is a single cluster on day one. See ADR-0003.

## Consequences

### Positive

- Newcomers (humans or LLMs) read one short document and have the platform's worldview.
- Later ADRs do not relitigate principles; they cite them.
- "Strong template" is a checkable property: an ADR either follows the principles or amends them.

### Negative / Risks

- The thesis ("100 services / 8 engineers / self-host") is the load-bearing assumption. If team size or hosting model changes materially, several later ADRs need re-evaluation. This is acknowledged, not mitigated.
- Strong opinions reduce flexibility. Teams that need to deviate must either justify with a new ADR or fork the template.

### Follow-ups

None — this ADR is the root.

## Rules

- An ADR exists for every decision that binds more than one service or is hard to reverse.
- An ADR follows the structure: Context → Decision drivers → Considered options → Decision → Consequences → Rules.
- Rejected options in an ADR are 2–4 lines each; deeper rationale lives in the PR.
- An accepted ADR is binding on every service. Per-service deviation requires a new ADR.
- A decision is unambiguous: no "or Y in some cases" without a measurable trigger.
- Nothing in the platform is "temporary." Either commit, or defer behind a hard trigger.
- Configuration is files in this repo. UI-only state is not allowed for anything reconciled into a cluster.
- Generated code is committed and drift-checked in CI.
- Service-to-service communication is HTTP via generated OpenAPI clients. Direct database, code, or workflow coupling across services is forbidden.
- Local development uses the same Helm charts, container images, and commands as production. Topology may differ; interface may not.
