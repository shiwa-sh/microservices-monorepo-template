# ADR-0502: Alerting & On-Call

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0307](0307-outbound-email.md), [ADR-0500](0500-observability.md), [ADR-0501](0501-operator-uis-and-dashboards.md)

## Context

[ADR-0500](0500-observability.md) ships alert rules as committed Prometheus rule files and stops there. A firing alert appears on the Prometheus alerts page and as the `ALERTS` series, and reaches no one. Overnight, that is indistinguishable from no alerting.

[ADR-0000](0000-platform-foundations.md) ranks alert routing and on-call **second** to concede when moving down axis B, on the grounds that no mature self-hosted escalation layer exists. That ranking conflates two concerns with different answers.

| Concern | Question | Self-hosted answer |
| --- | --- | --- |
| **Routing** | which humans and systems does this alert reach, deduplicated, grouped, and silenced during maintenance? | mature and standard |
| **Escalation** | who is on call right now, what happens when they do not acknowledge in five minutes, and how does a phone ring? | thin, and the reason for ADR-0000's ranking |

Routing is a solved component the platform declines to run. Escalation is a genuine gap. Deciding them together is what has left the platform with neither.

## Decision drivers

1. **An alert that reaches no one is not an alert.** Routing is the minimum that makes [ADR-0500](0500-observability.md)'s rule files real.
2. **Configuration in the repository** ([ADR-0000](0000-platform-foundations.md), principle 1). Routes, receivers, and silences are files.
3. **One primitive per concern** (principle 5). Alerts evaluate in one place, not in two.
4. **Thinnest viable platform** (principle 2). Routing must not arrive as an incident-management suite.
5. **The escalation exit is honest** (principle 3). Where the self-hosted answer is thin, that is stated rather than papered over.

## Considered options

### Where alerts evaluate and route

| Option | Rules as files | Routing, grouping, silencing | Weight | Verdict |
| --- | --- | --- | --- | --- |
| **Prometheus rules + Alertmanager** | native rule files, already shipped | Alertmanager's routing tree, as a committed config | one Go binary, no datastore | **Chosen.** It is the receiver the rule files were always written for |
| Grafana unified alerting | rules become Grafana objects, provisionable as YAML | built into Grafana, already Core | none — Grafana is already running | Zero marginal weight and rejected on principle 5: alert state moves into Grafana's database, and evaluation splits between two engines while [ADR-0500](0500-observability.md)'s rule files still exist |
| Rule files with no receiver — the honest baseline | yes | none | none | An alert that evaluates and reaches nobody. Driver 1 rules it out |

### Escalation and paging

| Option | Self-hosted | Rotation, escalation, acknowledgement | Reaches a phone | Verdict |
| --- | --- | --- | --- | --- |
| **A managed paging service, reached by webhook** | no | yes | yes | **Chosen as the sanctioned concession**, exactly where [ADR-0000](0000-platform-foundations.md) ranks it |
| Grafana OnCall | **no longer** | yes | it never did without Grafana's cloud | The self-hosted answer the field used to have. Its open-source distribution entered maintenance in March 2025 and was **archived in March 2026**, with the repository read-only and phone and SMS delivery withdrawn from OSS users. It is the direct evidence behind ADR-0000's ranking of this component |
| Keep | yes | alert enrichment, correlation, and workflows; not a rotation calendar | no | Answers the tier above this one — deduplicating and enriching alerts — and leaves "whose phone rings at 03:00" unanswered |
| LinkedIn Oncall | yes | **rotation calendars only** | no | A scheduling system with no alert path. Pairing it with a delivery tool rebuilds the product from two halves and an integration nobody maintains |
| ntfy or Gotify as a receiver | yes | none — delivery only | push notification, not a call | The honest self-hosted delivery floor. It moves a notification to a device without knowing who is on duty or noticing that nobody acknowledged |
| Email and chat only | yes | none | no | Sufficient where no rotation exists, and honest about what it is not. It is the template default |
| Build a rotation and escalation service | yes | whatever we write | whatever we integrate | An incident-management product, not platform glue |

**There is no longer a credible self-hosted escalation layer**, which is a stronger statement than this ADR could previously make and is why driver 5 exists. The concession is not a preference between comparable options; it is the absence of one.

## Decision

| Concern | Decision |
| --- | --- |
| Evaluation | **Prometheus**, from the committed rule files in [ADR-0500](0500-observability.md). Grafana alerting is not used |
| Routing | **Alertmanager**, joining Core. Its routing tree, receivers, inhibitions, and silences are committed files reconciled by Argo ([ADR-0201](0201-gitops.md)) |
| Default receivers | **email** through [ADR-0307](0307-outbound-email.md), and a **generic webhook** receiver that ships wired to nothing |
| Severity | every rule carries `severity: page` or `severity: ticket`. `page` routes to the webhook, `ticket` routes to email |
| Escalation | **not shipped.** The webhook receiver is the seam a paging service attaches to |
| Silences | a maintenance window is a committed silence, not a click in the Alertmanager UI (principle 1) |

**`severity: page` is a claim about a human.** A rule carries it only when a person must act within minutes. Every other rule is `ticket`. Without that discipline the routing tree is decoration, because a receiver that fires forty times a night is muted by its recipient within a week.

The test for which one a rule carries is [Google SRE's symptom rule](https://sre.google/sre-book/monitoring-distributed-systems/): page on what the user experiences, ticket on what merely explains it. A saturated queue is a `ticket`; the latency it eventually causes is a `page`. Naming the test gives a reviewer something to apply rather than a judgement to make.

Error budgets are the other half of that framing and are **not** adopted — they need an SLO per service, which nothing here produces ([ADR-0601](0601-testing-strategy.md)).

### On-call rotation is deferred, with the seam built

| Field | Value |
| --- | --- |
| **Trigger** | the platform commits to a response-time obligation outside working hours — an availability target with consequences, or the first paying customer contract that names one |
| **Seam** | ✓ Alertmanager's webhook receiver. Attaching a paging service is a receiver's URL and a credential; alert rules, severities, and the routing tree are unchanged |
| **Cost if adopted late** | overnight incidents are found in the morning. Bounded by the `page`/`ticket` split already being in the rules, so nothing is re-authored when the pager arrives |

This is a **deferral, not a bet**: the seam exists, and it is the receiver interface every paging vendor implements.

## Consequences

### Positive

- Alert rules reach a destination, which makes [ADR-0500](0500-observability.md)'s rule files load-bearing instead of decorative.
- The routing tree, silences, and severities are reviewable files, so an alert's destination is a diff.
- The escalation gap is stated in one place with a measurable trigger, rather than implied by an absence.

### Negative / Risks

- **Alertmanager joins Core**, adding a component whose own failure is silent. Its `Watchdog` alert — a rule that always fires and is expected to arrive continuously — is the standard answer, and a missing Watchdog is what a paging service watches for once one exists.
- **Nothing pages anyone until the trigger fires.** This platform detects overnight incidents in the morning. That is a deliberate position, not an oversight: a rotation nobody is rostered onto is a page that wakes no one.
- **Email as a `ticket` receiver depends on [ADR-0307](0307-outbound-email.md).** An outbound-mail failure degrades alerting, so mail-path alerts route to the webhook rather than to email.
- **Alert fatigue is the failure mode**, and no component prevents it. The `page`/`ticket` rule is review-enforced, which is weaker than a linter.

## Rules

- Alerts evaluate in Prometheus from committed rule files. Grafana-managed alert rules are not used.
- Alertmanager routes every alert. Its routing tree, receivers, and silences are committed files, never UI state ([ADR-0000](0000-platform-foundations.md), principle 1).
- Every alert rule carries `severity: page` or `severity: ticket`. `page` asserts a human must act within minutes.
- Maintenance silences are committed, time-bounded, and expire on their own.
- No on-call rotation is claimed until a paging receiver is attached to the webhook.
- Alerts about the outbound-mail path do not route through email.
