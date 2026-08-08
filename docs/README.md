# Documentation

Decisions are [ADRs](adr/README.md). Everything else is here, and earns its place by one rule:

> An ADR records a decision and its reasoning. A doc holds what an ADR must not — a **procedure to execute**, or **live state that changes without a decision changing**. Everything else is a link.

That is why there is no doc restating a stack choice, and why a runbook never argues for its own design.

## How-to — procedures

| Doc | Answers |
| --- | --- |
| [dev-loop](dev-loop.md) | How do I run and debug a service on my machine? |
| [database/migrations](database/migrations.md) | How do I write and apply a schema migration? |
| [database/major-upgrade](database/major-upgrade.md) | How do I move Postgres to a new major version? |
| [gitops/runbook](gitops/runbook.md) | How do I deploy, and what do I do when a sync is stuck? |
| [cluster/gitops-local](cluster/gitops-local.md) | How do I test uncommitted GitOps wiring? |
| [cluster/dr-runbook](cluster/dr-runbook.md) | How do I recover from full cluster loss? |
| [secrets/runbook](secrets/runbook.md) | How do I onboard, offboard, or rotate a key? |
| [gateway/runbook](gateway/runbook.md) | How do I change an edge rule or a rate limit? |
| [ops/break-glass](ops/break-glass.md) | How do I reach the dashboards when auth is down? |
| [ops/http-proxy](ops/http-proxy.md) | How do I work behind a corporate proxy? |
| [perf/runbook](perf/runbook.md) | How do I run a load test and read the result? |

## Reference — lookup

| Doc | Holds |
| --- | --- |
| [reference/local](reference/local.md) | Local ports, URLs, the frontend environment contract, the task index |
| [operational-surface](operational-surface.md) | Every platform component, tiered Core / Scale / Opt-in, plus the budget rule |
| [security-baseline](security-baseline.md) | The control → enforcing-artefact → verification matrix, and per-instance compliance escalation |
| [auth/jwt-validation](auth/jwt-validation.md) | The JWT validation rules, defined once |
| [temporal/long-running](temporal/long-running.md) | The registry of workflows whose wall-clock exceeds one deploy cycle |

## Where a fact lives

| Looking for | Go to |
| --- | --- |
| Why a tool was chosen, and what it beat | the owning ADR |
| The rule a reviewer enforces | that ADR's *Rules* section |
| The command to run | the how-to above |
| What is deployed right now | [operational-surface](operational-surface.md) |
| How this repo's prose is written | [ADR-0001](adr/0001-documentation-and-output-conventions.md) |
