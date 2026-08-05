# ADR-0011: Observability

- **Status:** Accepted
- **Date:** 2026-07-06
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

### Signals: all four instrumented on day one

Logs, metrics, traces, and continuous profiles. Every service is *instrumented* for all four from creation — partial instrumentation leads to per-team tribal knowledge ("we have metrics here, traces there"), which the per-service-cost principle cannot afford. All four ship a Grafana-stack backend: Loki/Prometheus/Tempo for logs, metrics and traces, and Pyroscope (fed by an Alloy agent) for profiles (see *Continuous profiling*).

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
4. Registers `pprof` HTTP endpoints on the admin port, so any profiler — or a human with `go tool pprof` — can reach a live process (see *Continuous profiling*).
5. Serves the two Kubernetes health endpoints on the admin port: `/livez` (liveness — shallow, always 200 while the process runs) and `/readyz` (readiness — deep, 200 only if every registered dependency check passes). Liveness never consults dependencies, so a dependency blip parks the pod out of Service rotation via `/readyz` **without** triggering a restart. The shared dependency wiring self-registers its checks (`dbmw.MustOpen` → `postgres`, `temporalmw.NewClient` → `temporal`), so each service gets exactly the checks for the dependencies it opens, with no per-service code. Servers probe both; workers (no inbound traffic) probe liveness only.
6. Returns a shutdown function that flushes all signals.

Service authors never touch the OTel SDK directly. Custom spans use `obs.StartSpan(ctx, "name")`; custom counters use `obs.Counter("name", ...)`. The library hides the SDK; the service hides nothing.

**Pre-wired middleware** that the service template imports automatically:

| Package               | Wraps                                     | Emits                                                        |
|-----------------------|-------------------------------------------|--------------------------------------------------------------|
| `libs/go/httpmw/`     | HTTP server & client                      | trace span, RED metrics, structured access log               |
| `libs/go/dbmw/`       | `pgx` tracer                              | DB span per query, statement metrics                         |
| `libs/go/temporalmw/` | Temporal client + worker interceptors     | workflow/activity spans + duration metrics                   |
| `libs/go/authmw/`     | identity-header reading ([ADR-0010](0010-auth.md)) | authz-failure metrics; user/org attributes on the active span |

A new service starts with full instrumentation by virtue of being created from `services/_template/`.

**What service developers add** beyond defaults:

- Business-meaningful log messages (middleware logs request lifecycle).
- Custom RED metrics for business KPIs (e.g. `payments_settlements_total{currency=...}`).
- Custom spans only when slow operations are not covered by HTTP/DB/Temporal middleware (rare).

### Backend: Grafana stack, monolithic mode

Self-hosted via Helm at `infra/helm/platform/observability/`:

- **Loki** — log storage.
- **Prometheus** — metrics storage. Single binary, local TSDB, native OTLP ingest.
- **Tempo** — trace storage, OTLP-native.
- **Grafana** — single UI for all three signals with cross-signal navigation (log → trace, metric → trace exemplar).

Prometheus is the metrics floor ([`docs/operational-surface.md`](../operational-surface.md), Core tier): one process, one PVC, no object-storage dependency for metrics. Loki and Tempo run in **single-binary monolithic mode** (`-target=all`), backed by the external bucket from [ADR-0003](0003-cluster-topology.md) for durability. One Helm release per backend, one Deployment or StatefulSet each.

**Metrics scale seam.** When metrics need multi-tenant isolation, HA, or long-retention object-storage-backed durability, the documented Scale swap is **Mimir** — the horizontally-scalable, Prometheus-compatible successor. It speaks the same query API and consumes the same dashboards and alert rules, so the swap changes storage config, not instrumentation. Loki and Tempo have the same seam into their own microservices modes when a single backend's ingest volume demands it. These swaps get their own ADR if one becomes a platform default.

**Sizing:**

- **prod:** Loki and Tempo each a single monolithic Deployment with 2 replicas for availability; Prometheus a single Deployment (its HA/sharding story is the Mimir Scale swap, not replica count); Grafana 2 replicas behind a service.
- **dev / staging:** single-replica everything. Failure tolerance for observability in non-prod is not worth the resource cost.

### Collection: a single OpenTelemetry Collector tier

**One tier:** OTel Collector as a DaemonSet on every node. Services send OTLP to `localhost:4317`. The agent batches, enriches with k8s resource attributes via the `k8sattributes` processor, applies head sampling, enforces resource limits, and exports directly to Loki, Prometheus (OTLP ingest), and Tempo. There is no separate gateway deployment.

Head sampling (errors at 100%, healthy traces at a low rate) runs on the agent. **Tail sampling** — holding spans to promote slow traces after the fact — needs the centralised gateway tier and is not deployed by default. A project whose trace volume justifies tail sampling adds the gateway tier behind a per-project flag; because services only ever emit to `localhost:4317`, adding it is purely additive and changes no service code.

**Logs follow a distinct path.** Services write structured JSON to **stdout**. The agent Collector reads `/var/log/pods/` via the `filelog` receiver, parses JSON, attaches k8s attributes, forwards. Stdout-first means logs survive even when the OTel SDK fails to start.

**Browser telemetry** from the frontend's Grafana Faro agent ([ADR-0014](0014-frontend.md)) enters through a `faro` receiver on the same Collector, fed by a Traefik route (`infra/gateway/frontend-observability.yaml`) for `/api/rum` (a vendor-neutral path — it names the concern, not the agent). The receiver emits web traces and RUM logs/events into the traces and logs pipelines, so browser and service signals share the same Tempo/Loki backends and trace IDs.

### Continuous profiling: Pyroscope + Alloy

`obs.Init` registers the `pprof` HTTP endpoints on the admin port (see above) — every Go service is profileable on day one at zero cost.

**Continuous profiling is Pyroscope + Alloy**, part of the observability chart: an Alloy agent scrapes each service's pprof admin endpoints and pushes profiles to Pyroscope; Grafana renders them via the Pyroscope datasource, as the flame-graph panel on the service-detail dashboard ([ADR-0025](0025-service-map-apm-ui.md)) — profiles live in the same pane as the SLO/RED/logs/traces that led you there. An eBPF node-agent profiler (e.g. Coroot's) was considered as a zero-instrumentation alternative and rejected with that suite as a whole ([ADR-0025](0025-service-map-apm-ui.md)); pprof endpoints already exist on every service, so scraping them adds no instrumentation cost and keeps profiles in the same backend family as the other three signals.

### Conventions

- **Logs are structured JSON to stdout.** `obs` configures `slog` accordingly. `fmt.Println` for diagnostics is forbidden.
- **Log levels:** `DEBUG` (off in prod), `INFO` (lifecycle), `WARN` (recoverable abnormalities), `ERROR` (failed operations). `FATAL`/`PANIC` only on startup.
- **Trace context** is propagated via W3C `traceparent` across HTTP and Temporal. Traefik and Oathkeeper preserve it at the edge ([ADR-0009](0009-api-gateway.md)).
- **Resource attributes** are set once by `obs.Init`: `service.name`, `service.version` and `service.build.sha` (both from the baked-in `buildinfo`, so telemetry self-reports the running build — [ADR-0013](0013-release-and-versioning.md)), `service.namespace`, `deployment.environment`.
- **Metric naming** follows OTel semantic conventions where they exist, else `<service>_<noun>_<unit>_<type>` (e.g. `payment_settlement_duration_seconds`).
- **HTTP RED comes from otelhttp's stable semantic-convention histogram** (`http_server_request_duration_seconds*`, with `http_response_status_code` and second-scale buckets). `httpmw` does not record a parallel custom duration metric; dashboards and alerts query the stable name only.
- **Workload CPU/memory come from the Collector's `kubeletstats` receiver** (`k8s_pod_cpu_usage`, `k8s_pod_memory_working_set_bytes`), so every pod appears in dashboards even when idle — pushed signals (RED/traces/app logs) only exist under live traffic, and that asymmetry is expected.
- **No PII in logs, metrics, traces, profiles.** Enforced by review; `libs/go/observability/redact/` provides safe formatters for user/org identifiers.
- **Sampling defaults:** head sampling at 100% for errors, 5% for healthy traces. Tail sampling at the gateway promotes slow traces to 100%. Service authors do not configure sampling.

### Cardinality discipline

High-cardinality labels (`user_id`, `request_id` as metric labels) destroy a metrics store. Three layered defences:

1. **API-level enforcement.** The `obs.Counter` / `obs.Histogram` API takes an allow-listed label set; arbitrary `WithAttributes(...)` is not exposed.
2. **Live cardinality alerts.** Prometheus's own TSDB series count feeds an alert that warns on Slack at 70% of the configured active-series ceiling and pages at 90%. Service-level alerts fire when any single metric's series count grows above its configured budget.
3. **Quarterly audit.** A Temporal `Schedule` opens a tracking issue summarising top series counts per service.

### Dashboards and alerts as code

- **Dashboards** are JSON files at `infra/observability/dashboards/*.json`, turned into the `grafana-dashboards` ConfigMap by a kustomize ArgoCD Application and mounted by Grafana's file provisioner.
- **Alerts** are native Prometheus rule files at `infra/observability/alerts/*.yaml`, turned into the `prometheus-alerts` ConfigMap by the parallel `prometheus-alerts` Application and mounted into Prometheus (`rule_files`). No Alertmanager ships in the default stack: firing alerts are visible on Prometheus `/alerts` and as the `ALERTS` series; routing (Slack/PagerDuty) is a per-project add-on.
- **Per-service defaults** ship in `services/_template/`: a default dashboard (RED metrics, error rate, latency p50/p95/p99, DB pool, Temporal worker) and a default alert set (5xx rate, p99 latency, DB connection saturation, worker queue lag). New services inherit them by name convention.

### Local development

Observability runs in the **full-platform tier** ([ADR-0016](0016-environment-parity.md)). `mise run cluster:full` brings up the real observability chart (`infra/helm/platform/observability`, the same one dev/staging/prod run) at a single replica, backed by in-cluster MinIO — so the backend wiring itself is validated, not just the service-side OTLP. The inner loop carries no collector — only the floor plus whatever components a service declares — since a natively-run service inspecting telemetry brings up the full tier.

Service code is unchanged between local and prod. The same `obs.Init` call works against the full-tier collector and the production stack; with neither running, the OTLP exporter simply has no collector to reach (`OTEL_SDK_DISABLED=true` in the inner-loop service values).

Reach the local stack through the edge (`grafana.ops.<host>`, behind the operator session — [ADR-0017](0017-url-and-domain-structure.md)) or, for raw access while iterating, `mise run dev:forward` port-forwards Grafana (`:3001`) and the Faro ingest (`:12347`) when the observability stack is up.

**Browser RUM (Faro) locally.** The frontend's Faro agent ([ADR-0014](0014-frontend.md)) POSTs beacons to the same-origin path `/api/rum`. In the cluster, Traefik forwards `/api/rum` to the Collector's `faro` receiver; but `next dev` runs on the host with no edge in front of it, so a dev-only Next route handler stands in (`apps/frontend/src/app/api/rum/route.ts`). With `FARO_COLLECT_URL=http://localhost:12347/collect` set (and `dev:forward` forwarding the full-tier Collector's `faro` receiver) it forwards beacons to it; with `FARO_COLLECT_URL` unset it silently returns `204` so the dev console isn't spammed with 404s. The handler 404s in production, matching the fact that Traefik — not the frontend pod — owns this path in the cluster.

## Consequences

### Positive

- Zero observability work per service. The template is the contract; services that follow it are fully observable for free.
- One UI, four signals, deep correlation.
- OTel-first means the backend is replaceable. A future swap to a hosted backend is a config change.
- Stdout-first logging makes logs the failure-mode-aware default.
- Dashboards and alerts as code — observability changes are reviewed like code.

### Negative / Risks

- **Backend operational cost is real**, even in monolithic mode. Loki, Prometheus, and Tempo each remain a system with its own runbook. Mitigated by single-binary deployments, off-cluster bucket durability for logs and traces, Prometheus's zero-dependency local TSDB, and single-replica sizing in non-prod.
- **Cardinality discipline.** Allow-listed labels, live alerts, and quarterly audits in combination.
- **No tail sampling by default.** Head sampling at the agent can drop a slow trace whose siblings looked healthy. Accepted at template scale; a project reintroduces the gateway tier for tail sampling when its trace volume justifies it, with no service-code change.
- **Profiling scrape scope.** Alloy scrapes pprof from the platform services' admin ports; third-party components without pprof endpoints are not profiled. Accepted — those components' health is covered by metrics/logs.

### Follow-ups

- `libs/go/observability/` (`Init`, helpers, redaction).
- `libs/go/{httpmw,dbmw,temporalmw,authmw}/` instrumentation (shared with earlier ADRs).
- `infra/helm/platform/observability/` Loki + Tempo (monolithic mode), Prometheus, Grafana, Pyroscope + Alloy (continuous profiling, [ADR-0025](0025-service-map-apm-ui.md)), and a single-tier OTel Collector DaemonSet. Mimir (metrics Scale swap) and the OTel Collector gateway tier are per-project add-ons (tail-sampling flag / Scale seam), not in the default release.
- `services/_template/` with default dashboard and alert YAMLs.
- `infra/observability/dashboards/_base.json` and `infra/observability/alerts/_base.yaml` shared baselines.
- Browser-beacon ingest: the `faro` receiver on the OTel Collector (`infra/helm/platform/observability/values.yaml`) + the `infra/gateway/frontend-observability.yaml` Traefik route ([ADR-0014](0014-frontend.md)); locally the same chart runs in the full tier (`cluster:full`).
- `docs/observability/conventions.md` (log levels, metric naming, redaction, sampling).
- Cardinality alerts in Prometheus rules; quarterly audit as a Temporal `Schedule`.

## Rules

- Every service initialises observability with `obs.Init(...)` from `libs/go/observability/`. Direct OTel SDK use in service code is forbidden.
- Every service imports the pre-wired middleware (`httpmw`, `dbmw`, `temporalmw`, `authmw`) by default.
- Logs are structured JSON to stdout. `fmt.Println` and unstructured loggers are forbidden.
- Log levels follow the conventions table above; `DEBUG` is off in prod.
- Metrics use the `obs.Counter` / `obs.Histogram` API with allow-listed labels. Arbitrary high-cardinality labels (user IDs, request IDs) are forbidden as metric attributes.
- Trace context is propagated via W3C `traceparent`. The edge (Traefik / Oathkeeper) preserves it; Temporal middleware propagates it.
- PII is never written to logs, metrics, traces, or profiles. Use `libs/go/observability/redact/` for identifiers.
- Sampling is configured centrally in the OTel Collector. Service authors do not set sampling rates. Tail sampling requires the gateway tier, which is a per-project add-on, not the default.
- Dashboards live as JSON files under `infra/observability/dashboards/`. Alerts live as YAML under `infra/observability/alerts/`. UI-only edits are not allowed; changes are PRs.
- The observability backend is the Grafana stack (Loki + Tempo in monolithic mode, Prometheus, Grafana) plus a single-tier OTel Collector, deployed from `infra/helm/platform/observability/`. Mimir (metrics Scale swap) and the collector gateway tier are per-project flags. Continuous profiling is Pyroscope + Alloy in the same chart ([ADR-0025](0025-service-map-apm-ui.md)). Alternate backends require an ADR.
- Long-term log and trace data lives in the off-cluster bucket from [ADR-0003](0003-cluster-topology.md); local PVCs hold hot cache only. Metrics live in Prometheus's local TSDB until a project takes the Mimir Scale swap.
