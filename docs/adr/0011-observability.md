# ADR-0011: Observability

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0001](0001-language-and-runtime.md), [ADR-0006](0006-temporal.md), [ADR-0008](0008-api-contracts.md)

## Context

At 100-service scale, observability is the only path by which a small team diagnoses problems across a fleet they cannot hold in their head.

We need a single answer to:

- **Logs, metrics, traces, and continuous profiles** from every service.
- **One backend** — engineers do not learn three UIs.
- **Self-host** for cost predictability and data control.
- **Zero observability work per service.** Adding a new service must not require new collector pipelines, new dashboards, or new alerts.
- **Cross-signal correlation.** A log line links to its trace; a trace links to its profile; a metric links to trace exemplars.

The "zero work per service" requirement is load-bearing. It is the difference between observability that scales and observability that is permanently in arrears.

## Decision drivers

1. **OpenTelemetry first.** Instrumentation is vendor-neutral; backends are replaceable.
2. **Pre-wired defaults in shared libraries.** Service code calls `obs.Init()` and gets everything.
3. **Self-host** ([ADR-0000](0000-platform-foundations.md)).
4. **Boring storage.** Mature, well-understood backends.

## Decisions

### Signals: all four, on day one

Logs, metrics, traces, and continuous profiles. Partial observability leads to per-team tribal knowledge ("we have metrics here, traces there"), which the per-service-cost principle cannot afford.

### Instrumentation: OpenTelemetry via shared libraries

A single `libs/go/observability/` package wires every signal in one call:

```go
func main() {
    shutdown, err := obs.Init(ctx, obs.Config{ServiceName: "payment"})
    if err != nil { log.Fatal(err) }
    defer shutdown(ctx)
    // ... rest of main
}
```

`obs.Init` does, in order:

1. Reads `OTEL_*` env vars (endpoint, sampling, resource attributes). No service-specific flags.
2. Configures global OTel `TracerProvider`, `MeterProvider`, `LoggerProvider` with OTLP exporters pointing at the local OTel Collector (`localhost:4317`).
3. Configures `slog` as the default logger with an OTel-aware handler that **automatically attaches `trace_id` and `span_id` from `context.Context`**.
4. Registers `pprof` HTTP endpoints on the admin port; the Pyroscope agent scrapes them.
5. Returns a shutdown function that flushes all signals.

Service authors never touch the OTel SDK directly. Custom spans use `obs.StartSpan(ctx, "name")`; custom counters use `obs.Counter("name", ...)`. The library hides the SDK; the service hides nothing.

**Pre-wired middleware** that the service template imports automatically:

| Package               | Wraps                                     | Emits                                                        |
|-----------------------|-------------------------------------------|--------------------------------------------------------------|
| `libs/go/httpmw/`     | HTTP server & client                      | trace span, RED metrics, structured access log               |
| `libs/go/dbmw/`       | `pgx` tracer                              | DB span per query, statement metrics                         |
| `libs/go/temporalmw/` | Temporal client + worker interceptors     | workflow/activity spans + duration metrics                   |
| `libs/go/authmw/`     | JWT validation ([ADR-0010](0010-auth.md)) | auth-failure metrics; user/org attributes on the active span |

A new service starts with full instrumentation by virtue of being created from `services/_template/`.

**What service developers add** beyond defaults:

- Business-meaningful log messages (middleware logs request lifecycle).
- Custom RED metrics for business KPIs (e.g. `payments_settlements_total{currency=...}`).
- Custom spans only when slow operations are not covered by HTTP/DB/Temporal middleware (rare).

### Backend: Grafana LGTM + Pyroscope

Self-hosted via Helm at `infra/helm/platform/observability/`:

- **Loki** — log storage.
- **Mimir** — metrics storage, Prometheus-compatible.
- **Tempo** — trace storage, OTLP-native.
- **Pyroscope** — continuous profile storage.
- **Grafana** — single UI for all four signals with cross-signal navigation (log → trace, trace → profile, metric → trace exemplar).

Object storage (the external bucket from [ADR-0003](0003-cluster-topology.md)) is the durability layer for Mimir, Tempo, Pyroscope. No ad-hoc local volumes for long-term data.

**Sizing:**

- **prod:** Loki/Mimir/Tempo each scaled with 3 replicas of read and write paths; Grafana 2 replicas behind a service.
- **dev / staging:** single-replica everything. Failure tolerance for observability in non-prod is not worth the resource cost.

### Collection: OpenTelemetry Collector, agent + gateway

Two-tier topology:

- **Agent tier:** OTel Collector as a DaemonSet on every node. Services send OTLP to `localhost:4317`. The agent batches, enriches with k8s resource attributes via the `k8sattributes` processor, and forwards to the gateway tier.
- **Gateway tier:** OTel Collector deployment (3 replicas in prod, 1 elsewhere). Performs tail sampling for traces (errors and slow traces kept; healthy traces sampled at a low rate), enforces resource limits, exports to Loki/Mimir/Tempo/Pyroscope.

**Logs follow a distinct path.** Services write structured JSON to **stdout**. The agent Collector reads `/var/log/pods/` via the `filelog` receiver, parses JSON, attaches k8s attributes, forwards. Stdout-first means logs survive even when the OTel SDK fails to start.

### Continuous profiling

- The **Pyroscope agent** scrapes `/debug/pprof/*` from each Go service. Labels include `trace_id`, so a slow trace links directly to the profile captured during it.
- **eBPF whole-node profiling** runs as a DaemonSet for language-agnostic baseline visibility — useful for escape-hatch services ([ADR-0001](0001-language-and-runtime.md)).
- Retention: 14 days.

### Conventions

- **Logs are structured JSON to stdout.** `obs` configures `slog` accordingly. `fmt.Println` for diagnostics is forbidden.
- **Log levels:** `DEBUG` (off in prod), `INFO` (lifecycle), `WARN` (recoverable abnormalities), `ERROR` (failed operations). `FATAL`/`PANIC` only on startup.
- **Trace context** is propagated via W3C `traceparent` across HTTP and Temporal. Tyk forwards it ([ADR-0009](0009-api-gateway.md)).
- **Resource attributes** are set once by `obs.Init`: `service.name`, `service.version` (from build), `service.namespace`, `deployment.environment`.
- **Metric naming** follows OTel semantic conventions where they exist, else `<service>_<noun>_<unit>_<type>` (e.g. `payment_settlement_duration_seconds`).
- **No PII in logs, metrics, traces, profiles.** Enforced by review; `libs/go/observability/redact/` provides safe formatters for user/org identifiers.
- **Sampling defaults:** head sampling at 100% for errors, 5% for healthy traces. Tail sampling at the gateway promotes slow traces to 100%. Service authors do not configure sampling.

### Cardinality discipline

High-cardinality labels (`user_id`, `request_id` as metric labels) destroy Mimir. Three layered defences:

1. **API-level enforcement.** The `obs.Counter` / `obs.Histogram` API takes an allow-listed label set; arbitrary `WithAttributes(...)` is not exposed.
2. **Live cardinality alerts.** Mimir's per-tenant active-series limit fires a Slack alert at 70% of the configured ceiling and pages at 90%. Service-level alerts fire when any single metric's series count grows above its configured budget.
3. **Quarterly audit.** A Temporal `Schedule` opens a tracking issue summarising top series counts per service.

### Dashboards and alerts as code

- **Dashboards** are JSON files at `infra/observability/dashboards/*.json`, provisioned via the Grafana operator from a ConfigMap.
- **Alerts** are Mimir alerting rules at `infra/observability/alerts/*.yaml`, routed to Slack and PagerDuty.
- **Per-service defaults** ship in `services/_template/`: a default dashboard (RED metrics, error rate, latency p50/p95/p99, DB pool, Temporal worker) and a default alert set (5xx rate, p99 latency, DB connection saturation, worker queue lag). New services inherit them by name convention.

### Local development

A `grafana/otel-lgtm` single-image bundle runs in k3d as part of `mise run dev:up` (and `mise run dev:up --minimal`). It accepts OTLP on `localhost:4317` and exposes Grafana on `localhost:3000` with Loki/Mimir/Tempo/Pyroscope pre-wired.

Service code is unchanged between local and prod. The same `obs.Init` call works against the local bundle and the production stack.

## Consequences

### Positive

- Zero observability work per service. The template is the contract; services that follow it are fully observable for free.
- One UI, four signals, deep correlation.
- OTel-first means the backend is replaceable. A future swap to a hosted backend is a config change.
- Stdout-first logging makes logs the failure-mode-aware default.
- Dashboards and alerts as code — observability changes are reviewed like code.

### Negative / Risks

- **LGTM operational cost is real.** Loki, Mimir, and Tempo are distributed systems with their own runbooks. Mitigated by Helm-managed deployments, off-cluster bucket durability, and single-replica sizing in non-prod.
- **Cardinality discipline.** Allow-listed labels, live alerts, and quarterly audits in combination.
- **Tail sampling at the gateway requires holding spans briefly.** Memory pressure under burst is mitigated by horizontal scaling of the gateway tier.
- **eBPF profiling has kernel-version sensitivities.** Mitigated by pinning kernels in the Ansible role from [ADR-0003](0003-cluster-topology.md).

### Follow-ups

- `libs/go/observability/` (`Init`, helpers, redaction).
- `libs/go/{httpmw,dbmw,temporalmw,authmw}/` instrumentation (shared with earlier ADRs).
- `infra/helm/platform/observability/` Loki, Mimir, Tempo, Pyroscope, Grafana, OTel Collector (agent + gateway).
- `services/_template/` with default dashboard and alert YAMLs.
- `infra/observability/dashboards/_base.json` and `infra/observability/alerts/_base.yaml` shared baselines.
- `docs/observability/conventions.md` (log levels, metric naming, redaction, sampling).
- Cardinality alerts in Mimir rules; quarterly audit as a Temporal `Schedule`.

## Rules

- Every service initialises observability with `obs.Init(...)` from `libs/go/observability/`. Direct OTel SDK use in service code is forbidden.
- Every service imports the pre-wired middleware (`httpmw`, `dbmw`, `temporalmw`, `authmw`) by default.
- Logs are structured JSON to stdout. `fmt.Println` and unstructured loggers are forbidden.
- Log levels follow the conventions table above; `DEBUG` is off in prod.
- Metrics use the `obs.Counter` / `obs.Histogram` API with allow-listed labels. Arbitrary high-cardinality labels (user IDs, request IDs) are forbidden as metric attributes.
- Trace context is propagated via W3C `traceparent`. Tyk forwards it; Temporal middleware propagates it.
- PII is never written to logs, metrics, traces, or profiles. Use `libs/go/observability/redact/` for identifiers.
- Sampling is configured centrally in the OTel Collector gateway. Service authors do not set sampling rates.
- Dashboards live as JSON files under `infra/observability/dashboards/`. Alerts live as YAML under `infra/observability/alerts/`. UI-only edits are not allowed; changes are PRs.
- The observability backend is the Grafana LGTM stack plus Pyroscope, deployed from `infra/helm/platform/observability/`. Alternate backends require an ADR.
- Long-term observability data lives in the off-cluster bucket from [ADR-0003](0003-cluster-topology.md). Local PVCs hold hot cache only.
