# Documentation

Decisions are [ADRs](adr/README.md). Everything else is here, and earns its place by one rule:

> An ADR records a decision and its reasoning. A doc holds what an ADR must not — a **procedure to execute**, or **live state that changes without a decision changing**. Everything else is a link.

That is why there is no doc restating a stack choice, and why a runbook never argues for its own design.

**New here?** [reading-path](reading-path.md) is the ordered entry: ninety minutes to navigable, then what each role reads next.

## How this tree is laid out

Genre decides the directory, so a path states what kind of document it points at before the file is opened ([ADR-0001](adr/0001-documentation-and-output-conventions.md)).

| Path | Genre | Holds |
| --- | --- | --- |
| `adr/` | explanation + reference | the decisions |
| `guide/` | how-to | a procedure someone executes |
| `reference/` | reference | a lookup, a registry, or live state |
| `docs/*.md` | entry and canonical registries | the way in, and the documents the ADR set cites by name as the only place a thing is recorded |

The root is deliberately small: [reading-path](reading-path.md) and [dev-loop](dev-loop.md) are where a reader starts, and [operational-surface](operational-surface.md), [adoption-path](adoption-path.md), and [security-baseline](security-baseline.md) are cited across the ADR set as the single home of the component inventory, the reduction order, and the control index. A file earns the root by being one of those two things.

Topic lives in the filename, not in a directory. A directory holding one file is a directory that buys nothing, and a `runbook.md` repeated once per topic is four files an editor cannot tell apart.

## How-to — procedures

| Doc | Answers |
| --- | --- |
| [dev-loop](dev-loop.md) | How do I run and debug a service on my machine? |
| [database-migrations](guide/database-migrations.md) | How do I write and apply a schema migration? |
| [postgres-major-upgrade](guide/postgres-major-upgrade.md) | How do I move Postgres to a new major version? |
| [gitops-runbook](guide/gitops-runbook.md) | How do I deploy, and what do I do when a sync is stuck? |
| [gitops-local](guide/gitops-local.md) | How do I test uncommitted GitOps wiring? |
| [disaster-recovery](guide/disaster-recovery.md) | How do I recover from full cluster loss? |
| [secrets-runbook](guide/secrets-runbook.md) | How do I onboard, offboard, or rotate a key? |
| [gateway-runbook](guide/gateway-runbook.md) | How do I change an edge rule or a rate limit? |
| [break-glass](guide/break-glass.md) | How do I reach the dashboards when auth is down? |
| [incident-management](guide/incident-management.md) | What counts as an incident, who does what, and what is owed afterwards? |
| [http-proxy](guide/http-proxy.md) | How do I work behind a corporate proxy? |
| [performance-runbook](guide/performance-runbook.md) | How do I run a load test and read the result? |

## Reference — lookup

| Doc | Holds |
| --- | --- |
| [local-environment](reference/local-environment.md) | Local ports, URLs, the frontend environment contract, the task index |
| [operational-surface](operational-surface.md) | Every platform component, tiered Core / Scale / Opt-in, with its recurring obligation, plus the budget rule |
| [adoption-path](adoption-path.md) | What to give up and in what order when the floor exceeds capacity, and what it costs to take back |
| [security-baseline](security-baseline.md) | Every security control and the mechanism enforcing it. **Generated** from the owning ADRs' Rules sections |
| [system-view](reference/system-view.md) | What runs, how a request moves through it, and where identity enters. One page |
| [risk-register](reference/risk-register.md) | Every accepted risk in the set, ranked, with its compensating control and its trigger |
| [deferral-register](reference/deferral-register.md) | Every deferral's trigger, and whether a machine can see it fire |
| [detection-latency](reference/detection-latency.md) | Per failure class: what detects it, and how long that takes |
| [threat-model](reference/threat-model.md) | The adversaries each control is against, over the three boundaries |
| [cost-model](reference/cost-model.md) | The shape of the bill and what appears on it, priced by the adopter |
| [rules-index](reference/rules-index.md) | Every rule in the set with its enforcement. **Generated** |
| [per-instance-hardening](reference/per-instance-hardening.md) | What a project turns on for its own risk profile or compliance framework |
| [asvs-verification](reference/asvs-verification.md) | The ASVS L2 claim, per concern, with the date each was last examined |
| [upstream-status](reference/upstream-status.md) | Third-party facts the decisions rest on, with the date each was verified |
| [jwt-validation](reference/jwt-validation.md) | The JWT validation rules, defined once |
| [long-running-workflows](reference/long-running-workflows.md) | The registry of workflows whose wall-clock exceeds one deploy cycle |

## Where a fact lives

| Looking for | Go to |
| --- | --- |
| Why a tool was chosen, and what it beat | the owning ADR |
| The rule a reviewer enforces | that ADR's *Rules* section |
| The command to run | the how-to above |
| What is deployed right now | [operational-surface](operational-surface.md) |
| What it costs to keep a component running | [operational-surface](operational-surface.md), *Recurring obligation* |
| Whether we can run less than this | [adoption-path](adoption-path.md) |
| What this platform accepts going wrong | [risk-register](reference/risk-register.md) |
| How long until we notice | [detection-latency](reference/detection-latency.md) |
| What we are waiting for, and who watches | [deferral-register](reference/deferral-register.md) |
| What the shape of the system is | [system-view](reference/system-view.md) |
| Whether an upstream fact still holds | [upstream-status](reference/upstream-status.md) |
| How this repo's prose is written | [ADR-0001](adr/0001-documentation-and-output-conventions.md) |
