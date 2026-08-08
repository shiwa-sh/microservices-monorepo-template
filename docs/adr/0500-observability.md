# ADR-0500: Observability

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0100](0100-language-and-runtime.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0200](0200-cluster-topology.md), [ADR-0302](0302-temporal.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0400](0400-frontend.md), [ADR-0501](0501-operator-uis-and-dashboards.md), [ADR-0502](0502-alerting-and-on-call.md), [ADR-0503](0503-error-tracking.md), [ADR-0600](0600-local-development-loop.md)

## Context

Observability is the only path by which a platform team diagnoses problems across a fleet it cannot hold in its head ([ADR-0000](0000-platform-foundations.md)).

This ADR answers: which signals, which backend, how they are collected, and what a service author must do.

**The "zero work per service" requirement is load-bearing.** It is the difference between observability that scales with the fleet and observability that is permanently in arrears. Adding a service must not require a new collector pipeline, a new dashboard, or a new alert.

## Decision drivers

1. **Configuration lives in the repository** ([ADR-0000](0000-platform-foundations.md), principle 1). Dashboards and alerts are files, reviewable and reconciled.
2. **OpenTelemetry first.** Instrumentation is vendor-neutral, so backends stay replaceable.
3. **Pre-wired defaults in shared libraries.** Service code makes one call and gets everything.
4. **Self-host** ([ADR-0000](0000-platform-foundations.md), principle 3).
5. **Boring storage.** Mature, well-understood backends.
6. **Cross-signal correlation.** A log links to its trace, a trace to its profile, a metric to trace exemplars.

## Considered options

| Option | Dashboards and alerts as code | Signals covered | Query language | Verdict |
| --- | --- | --- | --- | --- |
| **Grafana stack — Loki, Prometheus, Tempo, Pyroscope** | **JSON and YAML files in git**, reconciled by Argo | all four, plus browser RUM through a Faro receiver | PromQL, LogQL, TraceQL — portable | **Chosen** |
| SigNoz | **UI-only click-ops state in ClickHouse** | three; no continuous profiling | its own | Consolidating four components into one is genuinely attractive at a fixed operational budget, which is why this was reopened. It fails driver 1 outright — the same driver that rejects Coroot ([ADR-0501](0501-operator-uis-and-dashboards.md)) — and additionally loses continuous profiling and the Faro RUM receiver |
| OpenObserve | **UI-only click-ops state** | three | its own | Same failure as SigNoz on driver 1, with a non-portable query language on top |
| Elastic stack | index templates and dashboards exportable, and the working source of truth is the UI | three | its own | Heavier operationally than the whole Grafana stack, and the licence history is a governance risk |
| One vendor's managed platform | n/a | all | proprietary | Excluded by driver 4 |

The deciding property is the same one that decided identity ([ADR-0304](0304-identity-and-authorization.md)) and the operator UIs: **a component whose configuration lives in its own database is invisible to review and to Argo.** Applying that to observability but not to identity, or the reverse, would be the inconsistency.

## Decision

### All four signals, instrumented on day one

Logs, metrics, traces, and continuous profiles. Every service is instrumented for all four from creation. Partial instrumentation produces per-team tribal knowledge — metrics here, traces there — which the per-service cost principle cannot absorb.

### Instrumentation: one call

`libs/go/observability/` wires every signal:

```go
func main() {
    shutdown, err := obs.Init(ctx, obs.Config{ServiceName: "payment"})
    if err != nil { log.Fatal(err) }
    defer shutdown(ctx)
}
```

`obs.Init` does six things in order:

1. Reads `OTEL_*` environment variables. No service-specific flags.
2. Configures the global tracer, meter, and logger providers with OTLP exporters pointing at the node-local collector.
3. Configures `slog` with an OTel-aware handler that **attaches `trace_id` and `span_id` from the context automatically**.
4. Registers `pprof` endpoints on the admin port, so any profiler or a human reaches a live process.
5. Serves `/livez` and `/readyz` on the admin port.
6. Returns a shutdown function that flushes all signals.

**Liveness never consults dependencies**, so a dependency blip parks the pod out of Service rotation through `/readyz` **without** triggering a restart. The shared dependency wiring self-registers its checks, so each service gets exactly the checks for the dependencies it opens with no per-service code. Servers probe both; workers probe liveness only.

Service authors never touch the OTel SDK. Custom spans and counters go through `obs` helpers.

**Pre-wired middleware**, imported by the service template automatically:

| Package | Wraps | Emits |
| --- | --- | --- |
| `libs/go/httpmw/` | HTTP server and client | trace span, RED metrics, structured access log |
| `libs/go/dbmw/` | the `pgx` tracer | a span per query, statement metrics |
| `libs/go/temporalmw/` | Temporal client and worker interceptors | workflow and activity spans, duration metrics |
| `libs/go/authmw/` | identity-header reading | authz-failure metrics, user and org attributes on the active span |

**What a service author adds** beyond the defaults: business-meaningful log messages, custom RED metrics for business KPIs, and custom spans only where a slow operation is not already covered by the middleware.

### Backend

| Component | Role | Storage |
| --- | --- | --- |
| Loki | logs | monolithic mode, backed by the external bucket |
| Tempo | traces | monolithic mode, backed by the external bucket |
| Prometheus | metrics | single binary, local TSDB, native OTLP ingest — no object-storage dependency |
| Pyroscope | profiles | see below |
| Grafana | one UI for all signals, with cross-signal navigation | — |

Sizing: production runs Loki and Tempo at two replicas each and Grafana at two, with Prometheus single — its HA story is the Mimir swap, not replica count. Non-prod runs single replicas; failure tolerance for observability there is not worth the resources.

**Scale seam.** When metrics need multi-tenant isolation, HA, or long-retention durability, the swap is **Mimir**: Prometheus-compatible, same query API, same dashboards, same alert rules, so the swap changes storage configuration rather than instrumentation. Loki and Tempo have the same seam into their own microservices modes.

### Collection: one collector tier

The OTel Collector runs as a **DaemonSet** on every node. Services send OTLP to `localhost:4317`. The agent batches, enriches with Kubernetes resource attributes, applies head sampling, enforces resource limits, and exports directly to the backends. There is no gateway deployment.

**Deferred: tail sampling.** **Trigger:** trace volume justifying holding spans to promote slow traces after the fact. **Seam:** services only ever emit to `localhost:4317`, so adding the gateway tier is a deploy, not a code change.

**Logs follow a distinct path.** Services write structured JSON to **stdout**, and the agent reads the pod log files, parses, attaches attributes, and forwards. Stdout-first means logs survive even when the OTel SDK fails to start.

**Browser telemetry** from the frontend's Faro agent ([ADR-0400](0400-frontend.md)) enters through a `faro` receiver on the same collector, fed by a Traefik route on a vendor-neutral path that names the concern rather than the agent. The receiver emits web traces and RUM events into the same pipelines, so browser and service signals share backends and trace ids.

### Continuous profiling

`obs.Init` already registers pprof endpoints, so every Go service is profileable at zero cost. An **Alloy** agent scrapes those endpoints and pushes to **Pyroscope**, and Grafana renders the result as the flame-graph panel on the service-detail dashboard ([ADR-0501](0501-operator-uis-and-dashboards.md)) — profiles sit in the same pane as the signals that led you there.

An eBPF node-agent profiler was considered as a zero-instrumentation alternative and rejected with its suite as a whole ([ADR-0501](0501-operator-uis-and-dashboards.md)). The pprof endpoints already exist, so scraping them adds no instrumentation cost and keeps profiles in the same backend family.

### Conventions

| Concern | Rule |
| --- | --- |
| Log format | structured JSON to stdout. `fmt.Println` for diagnostics is not used |
| Log levels | `DEBUG` off in production, `INFO` for lifecycle, `WARN` for recoverable abnormalities, `ERROR` for failed operations. `FATAL` and `PANIC` only at startup |
| Trace context | W3C `traceparent` across HTTP and Temporal. Traefik and Oathkeeper preserve it at the edge |
| Resource attributes | set once by `obs.Init`: service name, version and build SHA from the baked-in build info ([ADR-0103](0103-release-and-versioning.md)), namespace, and environment |
| Metric naming | OTel semantic conventions where they exist, else `<service>_<noun>_<unit>_<type>` |
| HTTP RED | the stable semantic-convention histogram. No parallel custom duration metric is recorded, and dashboards query the stable name only |
| Workload CPU and memory | the collector's `kubeletstats` receiver, so **every pod appears even when idle**. Pushed signals exist only under live traffic, and that asymmetry is expected |
| PII | never in logs, metrics, traces, or profiles. `libs/go/observability/redact/` provides safe formatters |
| Sampling | head sampling at 100% for errors and 5% for healthy traces, configured centrally. Local development overrides healthy to 100%, so a developer sees the request they made. Service authors do not set rates |

### Cardinality discipline

High-cardinality labels destroy a metrics store. Three layered defences:

1. **API-level enforcement.** The `obs` metric helpers take an allow-listed label set; arbitrary attributes are not exposed.
2. **Live alerts.** The TSDB series count feeds an alert warning at 70% of the active-series ceiling and paging at 90%. Service-level alerts fire when a single metric grows past its budget.
3. **Quarterly audit.** A Temporal `Schedule` opens a tracking issue summarising top series counts per service.

### Dashboards and alerts as code

| Artefact | Source | Delivery |
| --- | --- | --- |
| Dashboards | JSON at `infra/observability/dashboards/` | a ConfigMap mounted by Grafana's file provisioner |
| Alerts | native Prometheus rule files at `infra/observability/alerts/` | a ConfigMap mounted into Prometheus |

Routing, grouping, and silencing are **Alertmanager**'s, decided in [ADR-0502](0502-alerting-and-on-call.md). Alerts evaluate here, from these files, and are never authored as Grafana-managed rules.

Per-service defaults ship in `services/_template/`: a default dashboard and a default alert set, inherited by name convention.

### Local development

Observability runs in the full tier ([ADR-0600](0600-local-development-loop.md)), using the same chart every environment runs at a single replica, so the backend wiring itself is validated rather than only the service-side export. The inner loop carries no collector.

Service code is unchanged between local and production: the same `obs.Init` works against either, and with neither running the exporter has no collector to reach.

**Browser RUM locally** needs a stand-in, because the dev server runs on the host with no edge in front of it. A dev-only route handler forwards beacons to a port-forwarded collector when configured, returns `204` when not so the console is not spammed, and 404s in production — matching the fact that Traefik, not the frontend pod, owns that path in the cluster.

## Consequences

### Positive

- Zero observability work per service. The template is the contract, and a service that follows it is fully observable.
- One UI, four signals, deep correlation.
- OTel-first keeps the backend replaceable.
- Stdout-first logging is failure-mode-aware by default.
- Dashboards and alerts are reviewed like code.

### Negative / Risks

- **Backend operational cost is real** even in monolithic mode; each backend remains a system with its own runbook. Mitigated by single-binary deployments, bucket durability, a zero-dependency local TSDB for metrics, and single-replica non-prod sizing.
- **Cardinality discipline depends on three layered defences**, none sufficient alone.
- **No tail sampling by default.** Head sampling can drop a slow trace whose siblings looked healthy. Accepted, with the gateway seam documented.
- **Profiling scrape scope is first-party only.** Third-party components without pprof endpoints are not profiled; their health is covered by metrics and logs.
- **Alerts route but nothing pages.** [ADR-0502](0502-alerting-and-on-call.md) ships the routing tree and leaves escalation as a recorded concession, so an overnight incident is found in the morning.

### Follow-ups

- `libs/go/observability/` with `Init`, helpers, and redaction.
- `libs/go/{httpmw,dbmw,temporalmw,authmw}/`.
- `infra/helm/platform/observability/` with the backends, Pyroscope and Alloy, and the collector DaemonSet. Mimir and the gateway tier are per-project add-ons rather than default releases.
- `services/_template/` default dashboard and alerts, plus the shared baselines.
- The `faro` receiver and its Traefik route, and the dev-only local shim.
- Cardinality alerts, and the quarterly audit as a Temporal `Schedule`.

## Rules

- Every service initialises observability with `obs.Init`. Direct OTel SDK use in service code is not permitted. `(CI: ci-lint)`
- Every service imports the pre-wired middleware by default. `(review-only)`
- Logs are structured JSON to stdout. `fmt.Println` and unstructured loggers are not used. `(CI: lint:log-vocab)`
- Log levels follow the conventions table, and `DEBUG` is off in production. `(review-only)`
- Metrics use the `obs` helpers with allow-listed labels. High-cardinality attributes are not used as metric labels. `(CI: ci-lint)`
- Trace context is propagated via W3C `traceparent`; the edge preserves it and Temporal middleware propagates it. `(review-only)`
- PII is never written to logs, metrics, traces, or profiles. `(review-only)`
- Sampling is configured centrally. Service authors do not set sampling rates. `(review-only)`
- Dashboards live as JSON and alerts as YAML under `infra/observability/`. UI-only edits are not made; changes are PRs. `(review-only)`
- The backend is the Grafana stack plus a single-tier collector. Alternate backends require an ADR. `(review-only)`
- Long-term log and trace data lives in the off-cluster bucket; local volumes hold hot cache only. `(review-only)`
- Every service exposes `/livez` and `/readyz` on the admin port, and liveness never consults dependencies. `(CI: lint:service-contract)`
