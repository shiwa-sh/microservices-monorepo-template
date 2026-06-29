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

### Signals: all four instrumented on day one

Logs, metrics, traces, and continuous profiles. Every service is *instrumented* for all four from creation — partial instrumentation leads to per-team tribal knowledge ("we have metrics here, traces there"), which the per-service-cost principle cannot afford. Three signals (logs, metrics, traces) ship a backend by default; the fourth (profiles) keeps its instrumentation latent and deploys its backend per-project (see *Continuous profiling*). The contract a service codes against is identical either way.

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
4. Registers `pprof` HTTP endpoints on the admin port; the Pyroscope agent scrapes them when profiling is enabled (see *Continuous profiling*).
5. Returns a shutdown function that flushes all signals.

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

### Backend: Grafana LGTM, monolithic mode

Self-hosted via Helm at `infra/helm/platform/observability/`:

- **Loki** — log storage.
- **Mimir** — metrics storage, Prometheus-compatible.
- **Tempo** — trace storage, OTLP-native.
- **Grafana** — single UI for all three signals with cross-signal navigation (log → trace, metric → trace exemplar).

Each backend runs in its **single-binary monolithic mode** (`-target=all`), not the read/write/backend microservices topology. One Helm release per backend, one Deployment each. Object storage (the external bucket from [ADR-0003](0003-cluster-topology.md)) is still the durability layer for Mimir and Tempo, so collapsing the compute tier costs no data: a future split to microservices mode is a values change, not a data migration.

Splitting any one backend into microservices mode is a per-project decision taken when that signal's ingest volume actually demands it — not a day-one cost. It gets its own ADR if it becomes the platform default.

**Sizing:**

- **prod:** Loki/Mimir/Tempo each a single monolithic Deployment with 2 replicas for availability; Grafana 2 replicas behind a service.
- **dev / staging:** single-replica everything. Failure tolerance for observability in non-prod is not worth the resource cost.

### Collection: a single OpenTelemetry Collector tier

**One tier:** OTel Collector as a DaemonSet on every node. Services send OTLP to `localhost:4317`. The agent batches, enriches with k8s resource attributes via the `k8sattributes` processor, applies head sampling, enforces resource limits, and exports directly to Loki/Mimir/Tempo. There is no separate gateway deployment.

Head sampling (errors at 100%, healthy traces at a low rate) runs on the agent. **Tail sampling** — holding spans to promote slow traces after the fact — needs the centralised gateway tier and is not deployed by default. A project whose trace volume justifies tail sampling adds the gateway tier behind a per-project flag; because services only ever emit to `localhost:4317`, adding it is purely additive and changes no service code.

**Logs follow a distinct path.** Services write structured JSON to **stdout**. The agent Collector reads `/var/log/pods/` via the `filelog` receiver, parses JSON, attaches k8s attributes, forwards. Stdout-first means logs survive even when the OTel SDK fails to start.

**Browser telemetry** from the frontend's Grafana Faro agent ([ADR-0014](0014-frontend.md)) enters through a `faro` receiver on the same Collector, fed by a Traefik route (`infra/gateway/frontend-observability.yaml`) for `/api/observability/faro/*`. The receiver emits web traces and RUM logs/events into the traces and logs pipelines, so browser and service signals share the same Tempo/Loki backends and trace IDs.

### Continuous profiling: latent by default

`obs.Init` registers the `pprof` HTTP endpoints on the admin port (see above) — every Go service is profileable on day one at zero cost. **No profile-storage backend (Pyroscope) and no eBPF node profiler are deployed by default.** Profiling is the least-reached-for of the four signals and the only one needing a privileged node-level agent (with kernel-version sensitivities).

A project chasing a real CPU or allocation problem sets `profiling: on`, which deploys Pyroscope storage and its pprof-scraping agent (labels include `trace_id`, so a slow trace links to the profile captured during it); the eBPF whole-node profiler ([ADR-0001](0001-language-and-runtime.md) escape-hatch services) is a further opt-in within that. Retention when enabled: 14 days.

### Conventions

- **Logs are structured JSON to stdout.** `obs` configures `slog` accordingly. `fmt.Println` for diagnostics is forbidden.
- **Log levels:** `DEBUG` (off in prod), `INFO` (lifecycle), `WARN` (recoverable abnormalities), `ERROR` (failed operations). `FATAL`/`PANIC` only on startup.
- **Trace context** is propagated via W3C `traceparent` across HTTP and Temporal. Traefik and Oathkeeper preserve it at the edge ([ADR-0009](0009-api-gateway.md)).
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

Observability runs in the **full-platform tier** ([ADR-0016](0016-environment-parity.md)). `mise run cluster:full` brings up the real observability chart (`infra/helm/platform/observability`, the same one dev/staging/prod run) at a single replica, backed by in-cluster MinIO — so the LGTM wiring itself is validated, not just the service-side OTLP. The lightweight `mise run cluster:up` inner loop carries only Postgres/Temporal/SpiceDB (no collector), since a natively-run service inspecting telemetry brings up the full tier.

Service code is unchanged between local and prod. The same `obs.Init` call works against the full-tier collector and the production stack; with neither running, the OTLP exporter simply has no collector to reach (`OTEL_SDK_DISABLED=true` in the inner-loop service values).

Reach the local stack through the edge (`grafana.ops.<host>`, behind the operator session — [ADR-0017](0017-url-and-domain-structure.md)) or, for raw access while iterating, `mise run dev:forward` port-forwards Grafana (`:3001`) and the Faro ingest (`:12347`) when the observability stack is up.

**Browser RUM (Faro) locally.** The frontend's Faro agent ([ADR-0014](0014-frontend.md)) POSTs beacons to the same-origin path `/api/observability/faro/collect`. In the cluster, Traefik forwards `/api/observability/faro/*` to the Collector's `faro` receiver; but `next dev` runs on the host with no edge in front of it, so a dev-only Next route handler stands in (`apps/frontend/src/app/api/observability/faro/collect/route.ts`). With `FARO_COLLECT_URL=http://localhost:12347/collect` set (and `dev:forward` forwarding the full-tier Collector's `faro` receiver) it forwards beacons to it; with `FARO_COLLECT_URL` unset it silently returns `204` so the dev console isn't spammed with 404s. The handler 404s in production, matching the fact that Traefik — not the frontend pod — owns this path in the cluster.

## Consequences

### Positive

- Zero observability work per service. The template is the contract; services that follow it are fully observable for free.
- One UI, four signals, deep correlation.
- OTel-first means the backend is replaceable. A future swap to a hosted backend is a config change.
- Stdout-first logging makes logs the failure-mode-aware default.
- Dashboards and alerts as code — observability changes are reviewed like code.

### Negative / Risks

- **LGTM operational cost is real**, even in monolithic mode. Loki, Mimir, and Tempo each remain a system with its own runbook. Mitigated by single-binary deployments, off-cluster bucket durability, and single-replica sizing in non-prod.
- **Cardinality discipline.** Allow-listed labels, live alerts, and quarterly audits in combination.
- **No tail sampling by default.** Head sampling at the agent can drop a slow trace whose siblings looked healthy. Accepted at template scale; a project reintroduces the gateway tier for tail sampling when its trace volume justifies it, with no service-code change.
- **Profiling is not on by default.** A team needing it must flip `profiling: on` rather than having data already captured. Accepted: the pprof endpoints are always live, so enabling it is a backend deploy, not a code change. eBPF profiling (when enabled) has kernel-version sensitivities, mitigated by pinning kernels in the Ansible role from [ADR-0003](0003-cluster-topology.md).

### Follow-ups

- `libs/go/observability/` (`Init`, helpers, redaction).
- `libs/go/{httpmw,dbmw,temporalmw,authmw}/` instrumentation (shared with earlier ADRs).
- `infra/helm/platform/observability/` Loki, Mimir, Tempo (monolithic mode), Grafana, and a single-tier OTel Collector DaemonSet. Pyroscope + eBPF profiler and the OTel Collector gateway tier are per-project add-ons (`profiling` / tail-sampling flags), not in the default release.
- `services/_template/` with default dashboard and alert YAMLs.
- `infra/observability/dashboards/_base.json` and `infra/observability/alerts/_base.yaml` shared baselines.
- Browser-beacon ingest: the `faro` receiver on the OTel Collector (`infra/helm/platform/observability/values.yaml`) + the `infra/gateway/frontend-observability.yaml` Traefik route ([ADR-0014](0014-frontend.md)); locally the same chart runs in the full tier (`cluster:full`).
- `docs/observability/conventions.md` (log levels, metric naming, redaction, sampling).
- Cardinality alerts in Mimir rules; quarterly audit as a Temporal `Schedule`.

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
- The observability backend is the Grafana LGTM stack (Loki, Mimir, Tempo) in monolithic mode plus a single-tier OTel Collector, deployed from `infra/helm/platform/observability/`. Profile storage (Pyroscope) and the collector gateway tier are per-project flags. Alternate backends require an ADR.
- Long-term observability data lives in the off-cluster bucket from [ADR-0003](0003-cluster-topology.md). Local PVCs hold hot cache only.
