# Deferral Register

Every deferral in the ADR set, in one table. The Trigger/Seam/Cost triad makes "later" falsifiable per decision, and this document makes it enumerable: a trigger nobody watches is a comment, and twenty triggers spread across twenty documents are twenty things nobody watches.

The reasoning stays in the owning ADR. What this adds is the column no ADR can answer for itself — **whether a machine can see the trigger fire.**

| Watched by | Meaning |
| --- | --- |
| **alert** | a rule already evaluates the condition, or could with no new signal |
| **query** | the data exists and nobody is asking; a scheduled query would see it |
| **event** | a person notices it happening — a contract, an incident, a decision elsewhere |

An **event** row is not a weaker deferral. Some triggers are commercial or organisational and no metric will ever carry them. What matters is that no row is mislabelled: a condition that could be alerted and is not is a gap, and a condition that only a human can see needs a human who knows to look.

## The register

| Deferred | Trigger | Watched by | Owner |
| --- | --- | --- | --- |
| Mimir, replacing Prometheus | active series cross the paging threshold and adding memory stops being the cheap answer, or retention exceeds one local TSDB | **alert** — the active-series rule already fires at 70% and 90% | [ADR-0500](../adr/0500-observability.md) |
| Tail sampling, via a collector gateway | a latency investigation fails because head sampling discarded the slow traces, twice | **event** | [ADR-0500](../adr/0500-observability.md) |
| PgBouncer in session mode | a service needs `LISTEN/NOTIFY` across statements, an advisory lock, or a session-scoped `SET` | **event** | [ADR-0300](../adr/0300-data.md) |
| Postgres row-level security | a service's tables begin carrying rows belonging to more than one organisation | **query** — a schema walk can see the tenant column arrive | [ADR-0300](../adr/0300-data.md) |
| Temporal Worker Controller | a workflow's wall-clock legitimately exceeds a deploy cycle, or a non-determinism failure reaches production twice | **alert** for the second half; **query** over `docs/temporal/long-running.md` for the first | [ADR-0302](../adr/0302-temporal.md) |
| A publish-subscribe bus | one event has three or more independent consumers whose identities the producer should not know, or a consumer needs to replay a stream | **event**. Labelled a **bet**: adopting rewrites the producers | [ADR-0302](../adr/0302-temporal.md) |
| A cluster segment in the slug grammar | a second cluster provisioned inside one environment | **query** — the inventory of clusters is committed | [ADR-0003](../adr/0003-naming-and-identifiers.md) |
| Per-consumer rate limiting | a project bills for API access, so a caller's quota differs by contract rather than by route | **event** | [ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md) |
| Directory provisioning (SCIM) | the first contract making directory-driven provisioning a condition of purchase | **event** | [ADR-0304](../adr/0304-identity-and-authorization.md) |
| An OpenFGA consistency token | a read-after-write staleness bug observed in production — a user completing a mutation and then being denied what it granted | **alert**, once a rule counts authorization denials that follow a mutation on the same resource | [ADR-0304](../adr/0304-identity-and-authorization.md) |
| Ops-tier token isolation | a non-first-party origin under the apex, or the product rendering user-supplied content to other users | **query** over the ingress set for the first; **event** for the second | [ADR-0306](../adr/0306-trust-tiers-and-urls.md) |
| A partner credential tier | a party outside the organisation is issued credentials against the API | **event** | [ADR-0306](../adr/0306-trust-tiers-and-urls.md) |
| The forge migration to Forgejo | the platform cluster serves its first environment | **event** | [ADR-0102](../adr/0102-source-control-and-ci.md) |
| SemVer for a published artifact | an artifact is published for a consumer outside this repository to pin | **event** | [ADR-0103](../adr/0103-release-and-versioning.md) |
| Escalation and paging | a response-time obligation outside working hours — an availability target with consequences, or a contract naming one | **event**. The largest row here: it is the trigger [`detection-latency.md`](detection-latency.md) is bounded by | [ADR-0502](../adr/0502-alerting-and-on-call.md) |
| Keyless signing | a first-party image is published for an external party to verify | **event** | [ADR-0104](../adr/0104-supply-chain-security.md) |
| GlitchTip | fingerprint triage becomes routine rather than incident work, or the novelty window misses a fault that reached a customer | **event** for the first; **query** for the second, comparing window-detected faults against reported ones | [ADR-0503](../adr/0503-error-tracking.md) |
| The `(admin)` route group, replacing Lowdefy | upstream releases stop for two quarters, or a security advisory goes unanswered for one | **query** — a scheduled check of the upstream release feed | [ADR-0401](../adr/0401-internal-admin.md) |
| Session replay | three distinct incidents in one quarter where a reported bug could not be reproduced from traces plus RUM logs | **event**, and it needs an incident record to count against ([`../ops/incident-management.md`](../ops/incident-management.md)) | [ADR-0700](../adr/0700-analytics.md) |
| ClickHouse | funnel-query p95 above 2s against the declared rollup window, or the events table sustaining more than about 10M rows per month | **alert** — both are already measurable | [ADR-0700](../adr/0700-analytics.md) |
| A second GitOps repository | values-bump PRs outnumber code PRs on `master`, or a review is opened against a diff more than half values bumps | **query** over merge history | [ADR-0201](../adr/0201-gitops.md) |
| Registry mirroring | an upstream registry rate-limits or removes a pinned digest the cluster depends on | **alert** — image pull failures are already a signal | [ADR-0105](../adr/0105-image-registry.md) |
| A transactional mail provider | a sustained delivery-failure or complaint rate that DMARC alignment and warmup do not resolve, or an uncleared blocklist listing | **query** over DMARC aggregate reports, which are reviewed on a cadence either way | [ADR-0307](../adr/0307-outbound-email.md) |
| Storybook | a component is edited by someone who does not run the app, or a visual regression reaches `master` twice | **event**. Labelled a **bet**: the stories do not exist to enable | [ADR-0400](../adr/0400-frontend.md) |
| Longhorn | any volume exceeding 50% of node disk | **alert** — node disk and PVC size are both collected | [ADR-0200](../adr/0200-cluster-topology.md) |
| Per-workload certificate identity | a service performing a monetary mutation, a second team owning a service, or an auditor requiring a CA chain | **event** | [ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md) |

## What the column shows

**Seven rows could be alerted and are not.** Those are the cheapest work in this document: the signal is already collected, and what is missing is a rule that fires when the condition the ADR wrote down comes true. A deferral whose trigger is alertable and unalerted is indistinguishable from a deferral with no trigger.

**Six rows are queries** — the data exists, and something has to ask. These belong on the same cadence as the cardinality audit and the restore rehearsal ([ADR-0500](../adr/0500-observability.md), [ADR-0200](../adr/0200-cluster-topology.md)), because that cadence already exists and already opens an issue.

**The rest are events**, and every one is commercial, organisational, or an incident. They are watched by whoever reads this register, which is why it is one document rather than twenty.

**Two rows are bets rather than deferrals** — the publish-subscribe bus and Storybook — because no seam exists. They are in this table for visibility and obey a different rule: a bet is adopted earlier than its trigger or paid for in a rewrite ([ADR-0000](../adr/0000-platform-foundations.md)).
