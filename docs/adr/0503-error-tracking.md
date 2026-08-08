# ADR-0503: Error Tracking

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0400](0400-frontend.md), [ADR-0500](0500-observability.md), [ADR-0502](0502-alerting-and-on-call.md)

## Context

Errors already reach the backends. Services emit `ERROR`-level structured logs into Loki, failing requests are head-sampled at full rate into Tempo, and the frontend's Faro agent forwards JS errors through the same collector ([ADR-0400](0400-frontend.md), [ADR-0500](0500-observability.md)). Finding an error is solved.

What no component covers is what a dedicated error tracker sells:

| Capability | Covered by the floor |
| --- | --- |
| Find an error, with its trace and logs | yes |
| Collapse repeat occurrences of one fault into one entry | no |
| Notice a fault never seen before | no |
| Know which release introduced it, and that a fixed one returned | no |
| Readable frames from minified frontend code | no |
| Assign, resolve, and reopen an issue | no — **and not wanted** |

The last row settles the shape of this decision. Issue lifecycle is a workflow product, and this platform does not want one: triage lives in the forge's issues ([ADR-0102](0102-source-control-and-ci.md)), and a second place to mark work done is a second backlog. What remains worth having is **grouping** and **novelty detection**, both of which are properties of the telemetry rather than of a workflow.

## Decision drivers

1. **Cross-signal correlation** ([ADR-0500](0500-observability.md), driver 6). An error must reach its trace, its logs, and its profile. A tracker with its own store is a signal island, and the jump back out of it is a copied trace id.
2. **One primitive per concern** ([ADR-0000](0000-platform-foundations.md), principle 5). A second SDK, a second agent in the browser, and a second ingest path for telemetry the collector already carries.
3. **The always-on floor is the budget** (principle 2). A component joins Core only when nothing on the floor covers its concern.
4. **Operational sovereignty** (principle 3). Stack traces and error messages are among the most sensitive telemetry a platform emits.
5. **Cardinality discipline** ([ADR-0500](0500-observability.md)). Grouping keys are unbounded by nature, and an unbounded label destroys a metrics store.

## Considered options

| Option | Added always-on components | Correlation with traces and logs | Licence | Verdict |
| --- | --- | --- | --- | --- |
| **OTel `exception.*` records in the existing backends** | **none** | native — same collector, same trace ids, same Grafana | — | **Chosen.** Buys grouping and novelty at no floor cost |
| **GlitchTip** | Django app, Postgres, Redis | a copied trace id, by hand | AGPL-3.0 | The strongest self-hosted tracker for this platform's size, and Sentry-SDK-compatible so the ingest protocol is an exit. **Deferred to a Scale swap**, not rejected — see the trigger below |
| Sentry, self-hosted | Kafka, ClickHouse, Snuba, Relay, Redis, Postgres, and several worker classes | same island | **BUSL/FSL — not OSI open source** | Rejected twice over. It brings a datastore fleet and a message bus to the floor for one concern, and its licence makes it a governance dependency rather than a component we own. Upstream supports Compose; Kubernetes is a community chart |
| Highlight.io | ClickHouse plus its own services | same island | Apache-2.0 | Genuinely open and genuinely capable, including session replay. It is a second observability platform beside the Grafana stack, which is the consolidation [ADR-0500](0500-observability.md) already refused when it rejected SigNoz |
| Sentry SaaS, Bugsnag, Rollbar | none | same island | proprietary | Fails principle 3, and stack traces are the payload most likely to carry user data. Would join the [ADR-0000](0000-platform-foundations.md) swap list rather than the floor |
| Do nothing | none | native | — | The honest baseline. Errors remain findable and ungroupable, so a fault that fires constantly looks identical to one that fired once |

**Why the two open trackers lose to a design rather than to a rival product.** Both GlitchTip and Highlight are good at what they do, and weight is not the whole objection.

An error tracker owns a *second copy* of telemetry the collector already carries, so the walk from an exception to the trace that produced it becomes a copy-paste between two systems. [ADR-0500](0500-observability.md) spent components specifically to avoid signal islands. Adding one back to gain grouping inverts that trade.

## Decision

**Errors are OpenTelemetry data, not a separate product.**

| Concern | Decision |
| --- | --- |
| Recording | a failure is recorded as an OTel exception with `exception.type`, `exception.message`, and `exception.stacktrace`, on the active span and as a log record. `libs/go/observability` and the frontend wiring do this; service authors do not hand-roll it |
| Grouping | services compute a stable **`error.fingerprint`** — a hash over the exception type and the top application frames, with line numbers, addresses, and vendor frames excluded so a cosmetic edit does not mint a new fault |
| Cardinality | `error.fingerprint` is **log structured metadata and a span attribute only**. It is never a Loki stream label and never a metric label ([ADR-0500](0500-observability.md)) |
| Counting | one bounded metric, `errors_total{service, kind}`, where `kind` is a small closed enumeration owned by the `obs` helpers. The fingerprint does not appear on it |
| Release attribution | every record carries the image tag and commit already stamped by [ADR-0103](0103-release-and-versioning.md), so "which deploy introduced it" is a filter |
| Frontend frames | source maps are retained as build artefacts per release and are **not** shipped to the browser. Symbolication is a deliberate step against the stored map, not an always-on service |
| Alerting | rate and burst alerts on `errors_total` route through [ADR-0502](0502-alerting-and-on-call.md). A dashboard groups by fingerprint over the log store |
| Triage | the forge's issues ([ADR-0102](0102-source-control-and-ci.md)). Nothing else tracks state on an error |

**Error messages carry no interpolated user data.** The same rule as structured logging ([ADR-0001](0001-documentation-and-output-conventions.md)): context is attributes. A stack trace is exported telemetry, and an interpolated identifier in a message is PII that no redaction layer downstream can find.

### Novelty detection is the weak edge

Grouping by fingerprint is a query. "This fingerprint has never been seen before" is state, and the floor holds no store for it.

The default is a scheduled comparison of the current window's fingerprints against the previous window, emitting a `ticket`-severity alert on a set difference. That catches a new fault within the window and misses one that appeared and stopped inside it. Stated plainly rather than presented as parity.

### GlitchTip is the Scale swap

| Field | Value |
| --- | --- |
| **Trigger** | fingerprint triage becomes routine work rather than incident work — a recurring session spent grouping errors by hand — or the window comparison above misses a fault that reached a customer |
| **Seam** | ✅ GlitchTip ingests the Sentry envelope protocol, and the collector can fan out to a second exporter. Adopting it adds a destination; it does not change how services record errors |
| **Cost if adopted late** | triage stays manual and novelty stays window-bound. Nothing is re-instrumented, because the `exception.*` records GlitchTip needs are the ones already emitted |

The seam is real and the cost of waiting is bounded, which makes this a deferral rather than a bet.

## Consequences

### Positive

- No component joins the floor, and no second SDK reaches the browser or the services.
- An error is one click from its trace, its logs, and its profile, because it never left the pipeline that carries them.
- The fingerprint rule makes grouping a property of the data, so it survives a change of backend.
- Adopting a tracker later is an exporter, not an instrumentation project.

### Negative / Risks

- **No issue lifecycle.** An error is not assigned, resolved, or reopened. Accepted deliberately: that work belongs in the forge, and a second backlog is worse than none.
- **Novelty detection is window-bound**, as above. This is the sharpest gap and the clearest trigger for the swap.
- **Frontend symbolication is a manual step** against a stored source map. A minified trace is unreadable until someone does it.
- **The fingerprint is ours to get right.** Too coarse merges distinct faults; too fine mints a new fault per release. It is centralised in the `obs` helpers so it is tuned in one place, and it carries unit tests.
- **Grouping is a query, not a landing page.** An engineer runs a dashboard rather than opening an inbox.

### Follow-ups

- `error.fingerprint` in `libs/go/observability` and in the frontend's Faro wiring, with tests over the normalisation.
- `errors_total{service, kind}` in the `obs` metric helpers, with `kind` allow-listed.
- An errors-by-fingerprint dashboard, and the scheduled window comparison behind the novelty alert.
- Source-map retention per release, and a symbolication task.

## Rules

- A failure is recorded as an OTel exception with `exception.type`, `exception.message`, and `exception.stacktrace`. No service carries a second error-reporting SDK. `(review-only)`
- `error.fingerprint` is a span attribute and log structured metadata. It is never a Loki stream label and never a metric label. `(review-only)`
- Error messages carry no interpolated user data; context is attributes ([ADR-0001](0001-documentation-and-output-conventions.md)). `(review-only)`
- Errors are not tracked as issues anywhere but the forge. No component holds resolution state on an error. `(review-only)`
- Source maps are build artefacts retained per release and are never served to the browser. `(review-only)`
