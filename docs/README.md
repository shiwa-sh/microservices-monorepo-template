# Documentation

Decisions are [ADRs](adr/README.md). Everything else is here, and earns its place by one rule:

> An ADR records a decision and its reasoning. A doc holds what an ADR must not — a **procedure to execute**, or **live state that changes without a decision changing**. Everything else is a link.

That is why there is no doc restating a stack choice, and why a runbook never argues for its own design.

**New here?** [reading-path](reading-path.md) is the ordered entry: ninety minutes to navigable, then what each role reads next.

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
| [ops/incident-management](ops/incident-management.md) | What counts as an incident, who does what, and what is owed afterwards? |
| [ops/http-proxy](ops/http-proxy.md) | How do I work behind a corporate proxy? |
| [perf/runbook](perf/runbook.md) | How do I run a load test and read the result? |

## Reference — lookup

| Doc | Holds |
| --- | --- |
| [reference/local](reference/local.md) | Local ports, URLs, the frontend environment contract, the task index |
| [operational-surface](operational-surface.md) | Every platform component, tiered Core / Scale / Opt-in, with its recurring obligation, plus the budget rule |
| [adoption-path](adoption-path.md) | What to give up and in what order when the floor exceeds capacity, and what it costs to take back |
| [security-baseline](security-baseline.md) | Every security control and the mechanism enforcing it. **Generated** from the owning ADRs' Rules sections |
| [reference/system-view](reference/system-view.md) | What runs, how a request moves through it, and where identity enters. One page |
| [reference/risk-register](reference/risk-register.md) | Every accepted risk in the set, ranked, with its compensating control and its trigger |
| [reference/deferral-register](reference/deferral-register.md) | Every deferral's trigger, and whether a machine can see it fire |
| [reference/detection-latency](reference/detection-latency.md) | Per failure class: what detects it, and how long that takes |
| [reference/threat-model](reference/threat-model.md) | The adversaries each control is against, over the three boundaries |
| [reference/cost-model](reference/cost-model.md) | The shape of the bill and what appears on it, priced by the adopter |
| [reference/rules-index](reference/rules-index.md) | Every rule in the set with its enforcement. **Generated** |
| [reference/per-instance-hardening](reference/per-instance-hardening.md) | What a project turns on for its own risk profile or compliance framework |
| [reference/asvs-verification](reference/asvs-verification.md) | The ASVS L2 claim, per concern, with the date each was last examined |
| [reference/upstream-status](reference/upstream-status.md) | Third-party facts the decisions rest on, with the date each was verified |
| [auth/jwt-validation](auth/jwt-validation.md) | The JWT validation rules, defined once |
| [temporal/long-running](temporal/long-running.md) | The registry of workflows whose wall-clock exceeds one deploy cycle |

## Where a fact lives

| Looking for | Go to |
| --- | --- |
| Why a tool was chosen, and what it beat | the owning ADR |
| The rule a reviewer enforces | that ADR's *Rules* section |
| The command to run | the how-to above |
| What is deployed right now | [operational-surface](operational-surface.md) |
| What it costs to keep a component running | [operational-surface](operational-surface.md), *Recurring obligation* |
| Whether we can run less than this | [adoption-path](adoption-path.md) |
| What this platform accepts going wrong | [reference/risk-register](reference/risk-register.md) |
| How long until we notice | [reference/detection-latency](reference/detection-latency.md) |
| What we are waiting for, and who watches | [reference/deferral-register](reference/deferral-register.md) |
| What the shape of the system is | [reference/system-view](reference/system-view.md) |
| Whether an upstream fact still holds | [reference/upstream-status](reference/upstream-status.md) |
| How this repo's prose is written | [ADR-0001](adr/0001-documentation-and-output-conventions.md) |
