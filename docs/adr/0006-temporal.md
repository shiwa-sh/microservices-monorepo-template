# ADR-0006: Durable Execution (Temporal)

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0001](0001-language-and-runtime.md)

## Context

The platform needs one answer to a family of related problems:

- Operations spanning **multiple services or external systems** that must not leave the system half-applied (registration, payment, refund).
- Operations needing **compensation on failure** (sagas).
- **Scheduled / periodic work** — daily reports, hourly reconciliations, cleanup.
- **Background / async work** — emails, thumbnails, indexing, fan-out notifications.
- **Authz-relevant mutations** ([ADR-0010](0010-auth.md)) that write to both the application database and the authorization datastore atomically-in-effect.

Without a single durable-execution platform, each grows its own ad-hoc machinery: outbox tables, dead-letter queues, retry loops, cron jobs, custom idempotency. At 100 services this is unmaintainable.

## Decision drivers

1. **One reliability primitive, not five.** No coexistence of outbox + DLQ + cron + ad-hoc retries with a workflow engine.
2. **Service boundaries stay HTTP/OpenAPI.** A workflow engine must not become the cross-service bus.
3. **Operationally cheap per workflow.** Adding a workflow must cost less than building the same reliability by hand.
4. **Self-host** ([ADR-0000](0000-platform-foundations.md)).

## Considered options

- **Temporal (self-hosted)** — durable execution as a first-class primitive; mature Go SDK; Postgres-backed; activities, child workflows, signals, queries, timers, schedules, sagas in one model.
- **Restate** — newer, simpler ops model, but young; SDK and saga ergonomics trail Temporal. Worth watching, not adopting.
- **Cadence** — Uber's predecessor to Temporal; less momentum. No upside over Temporal.
- **DIY (outbox + queue + cron + retry loops)** — re-implements Temporal poorly four times.
- **k8s Jobs / CronJobs / Argo Workflows** — pipeline DAGs, not business workflows. Retained for pure infrastructure tasks (DB vacuum, log rotation).
- **Temporal Cloud** — managed; out of scope by the self-host principle. Client SDK call sites are cloud-neutral, so future migration is a deploy change.

## Decision

**Self-hosted Temporal is the platform's durable-execution layer.** It is the single answer to: long-running business workflows, sagas, scheduled jobs, and background async work.

### Scope: when is something a workflow

A workflow is the right primitive when **any** of the following hold:

1. The operation has **2+ logical steps** where partial completion is a bad state.
2. The operation **touches more than one service or external system.**
3. The operation must **compensate on failure**, not just return an error.
4. The operation is **long-lived relative to a request** (the user cannot hold the connection open).
5. The operation must **survive process restarts** by design.

A workflow is the **wrong** primitive when the operation is a single atomic action inside one service. A `PATCH /profile { name }` is not a workflow.

Concrete classification:

| Operation                                                    | Workflow?                                    |
|--------------------------------------------------------------|----------------------------------------------|
| Register user (Kratos + orgs + authz tuples + welcome email) | Yes                                          |
| Checkout (reserve → charge → order → deduct → confirm)       | Yes                                          |
| Payment (often a child workflow of checkout)                 | Yes                                          |
| Refund (compensation, multi-system)                          | Yes                                          |
| Authz-relevant resource mutation (app DB + authz store)      | Yes — per [ADR-0010](0010-auth.md)           |
| Send transactional email                                     | Yes                                          |
| Generate thumbnail / index document                          | Yes                                          |
| Daily reconciliation job                                     | Yes — Temporal `Schedule`, not k8s `CronJob` |
| Update profile name                                          | No                                           |
| Add item to cart                                             | No                                           |
| List orders                                                  | No                                           |

### Architecture: co-located workflows, HTTP between services

Workflows, activities, and workers live **inside the service that owns the business process**.

```text
services/<service>/
├── openapi.yaml
├── cmd/{server,worker}/main.go
├── internal/
│   ├── handlers/        # HTTP handlers (from generated server stubs)
│   ├── workflows/       # workflows owned by this service
│   ├── activities/      # activities owned by this service
│   ├── domain/
│   └── store/
└── migrations/
```

**Process-owner rule.** A workflow lives in the service that owns the *business process*, not the service that owns the most data. "Register user" lives in `identity` (or a dedicated `onboarding`) even though it writes to `orgs` and the authz store. "Checkout" lives in `checkout` even though it calls `payment`, `inventory`, and `orders`. A service whose primary job is orchestrating other services is a legitimate shape, not a smell.

**Cross-service workflow invocation: HTTP only.**

1. The owning service exposes an HTTP endpoint that internally starts the workflow.
2. The caller invokes it via the generated client in `libs/go/sdks/<service>/`.
3. The response is `202 Accepted` with a workflow handle conforming to the `WorkflowHandle` schema declared in each service's OpenAPI `components` (see [ADR-0008](0008-api-contracts.md)).

A service never starts another service's workflow directly via the Temporal client; doing so would import the callee's workflow input struct (coupling) and bypass OpenAPI, Tyk, tracing, and auth.

**Waiting on a cross-service workflow.** Pick by need:

1. **Poll the handle.** The owning service exposes `GET /<resource>/{id}` returning `{status, result?}`. The caller's workflow polls with backoff.
2. **Webhook callback.** The caller passes `callback_url`; the owning service POSTs on completion; the caller's workflow waits on a Temporal signal raised by its own webhook handler.
3. **Fire-and-forget.** The caller doesn't need the result.

Direct Temporal signals across service boundaries are not permitted.

### Activity placement

| Scope of use                                          | Location                                                                                                                        | Notes                                                            |
|-------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------|
| One service's workflows only                          | `services/<service>/internal/activities/`                                                                                       | The 90% case.                                                    |
| Generic infrastructure (email, S3, metrics, webhooks) | `libs/go/temporal-activities/<concern>/`                                                                                        | Stateless and service-agnostic. No dependency on `services/...`. |
| Logically owned by another service                    | **Not shared.** Each caller writes a thin activity in its own `internal/activities/` wrapping `libs/go/sdks/<owning-service>/`. | The shared thing is the HTTP API.                                |

Sharing an activity *across services* by putting domain logic in `libs/` is a smell.

### Wall-clock: workflows complete within one deploy cycle

A workflow's wall-clock should fit inside one prod deploy cycle (~1 week per [ADR-0004](0004-gitops.md)). This keeps us out of `workflow.GetVersion` / patching ceremony.

Long-running workflows (subscription billing, multi-day onboarding nudges, inactivity timers) are permitted but each requires:

1. An entry in `docs/temporal/long-running.md` listing the workflow and its expected wall-clock.
2. A documented versioning plan using `workflow.GetVersion`.
3. Replay tests (`workflow.NewReplayer`) covering historical event histories in CI.

Liberal use of workflows for multi-step / compensable / cross-system operations; conservative use of long wall-clocks.

### Replacement of legacy patterns

- **Outbox tables → workflows.** The application DB write and the downstream effect are activities in the same workflow.
- **Event bus (NATS / Kafka) — not adopted.** HTTP + Temporal signals (via webhook callbacks where needed) cover cross-service notification. A true pub/sub fan-out need gets its own ADR.
- **k8s CronJobs → Temporal Schedules** for any business-meaningful periodic task. k8s CronJobs remain for pure infrastructure (DB vacuum, log rotation).
- **Background queues → workflows** on a `background-tq` task queue.

### Operational shape

- **Server:** Temporal monolith binary (all four roles in one process) deployed via Helm at `infra/helm/platform/temporal/`. Backed by the platform Postgres (separate database, same cluster).
- **Workers:** one worker deployment per service at `services/<service>/cmd/worker/`. Registers only the workflows and activities owned by that service. Ships and deploys with its service.
- **Task queues:** named per-service (e.g. `payments-tq`, `checkout-tq`), plus a shared `background-tq`.
- **Namespaces:** one per environment (`dev`, `staging`, `prod`).
- **History retention:** 30 days production, 7 days non-prod.
- **Local development:** `temporal server start-dev` invoked by `mise run dev:up`.

### Conventions

- **Workflow code is deterministic.** No `time.Now()`, no `math/rand`, no direct I/O, no goroutines (use `workflow.Go`). Side effects go through activities. A `workflowcheck` analyzer runs in CI.
- **Activities are idempotent.** Every activity tolerates being invoked twice with the same input. The workflow run ID + activity ID is the natural idempotency key for external calls.
- **Activity inputs and outputs are small** (kilobytes, not megabytes). Large payloads go through the off-cluster bucket from [ADR-0003](0003-cluster-topology.md); activities pass references.
- **Timeouts are explicit.** Default activity `StartToCloseTimeout = 30s`, default workflow `WorkflowExecutionTimeout = 1h`. Overrides require justification in the workflow file.
- **Errors are typed** via `temporal.NewApplicationError` with stable error types; retry policy keys off them.
- **Workflow IDs encode business intent.** `payment-{order_id}`, not `payment-{uuid}`. Idempotency is a property of the business operation.
- **No SDK calls outside** `services/<service>/internal/workflows/` and `…/internal/activities/`.

### Cross-cutting integrations

Authz dual-write discipline is [ADR-0010](0010-auth.md)'s; the `WorkflowHandle` response shape is [ADR-0008](0008-api-contracts.md)'s;
service-to-service JWTs on activity calls are [ADR-0010](0010-auth.md)'s; trace propagation through workflows and
activities is [ADR-0011](0011-observability.md)'s default. Nothing here overrides those.

## Consequences

### Positive

- One reliability primitive answers four problems. Engineers learn Temporal once; no second runtime to operate.
- Saga compensation is boring, the highest praise distributed transactions can receive.
- Outbox patterns disappear as a concept.
- Authz dual-write risk ([ADR-0010](0010-auth.md)) is structurally solved, not policed.
- Service boundaries remain HTTP / OpenAPI.

### Negative / Risks

- Non-determinism rules are a real cognitive tax. Mitigated by `workflowcheck`, code-review checklist, and PR template.
- Temporal server is critical infrastructure. Mitigated by HA Postgres and replay tests proving workflows tolerate restarts.
- Per-activity latency (typically tens of milliseconds) rules workflows out of sub-100ms request paths. The scope rule already excludes those.
- One worker deployment per service multiplies pod count. Accepted; preserves ownership.

### Follow-ups

- `infra/helm/platform/temporal/` deployment with Postgres backing.
- `libs/go/temporal/` shared client config, default retry policies, tracing middleware, replay-test scaffolding.
- `tools/scripts/dev-up.sh` brings up `temporal server start-dev` alongside other local infra.
- `golangci-lint` config including `workflowcheck`.
- `docs/temporal/long-running.md` registry (initially empty).
- Standard `202 Accepted` workflow-handle shape, declared inline as the `WorkflowHandle` schema in each service's `openapi.yaml` `components`.

## Rules

- Temporal is the only durable-execution mechanism in the platform. No coexisting outbox tables, DLQs, ad-hoc retry loops, or cron jobs for business-meaningful periodic work.
- A workflow exists if and only if the operation matches at least one of the five scope criteria above.
- Workflows, activities, and the worker for a service live under `services/<service>/`. There is no top-level workflow directory.
- Cross-service workflow invocation is HTTP through the generated client. Direct Temporal-client calls across service boundaries are forbidden.
- Cross-service result wait is one of: poll the handle, webhook callback, fire-and-forget. Direct cross-service Temporal signals are forbidden.
- Activities are placed by ownership: in-service for service-specific logic, in `libs/go/temporal-activities/` for stateless infrastructure, never shared across services as a domain wrapper.
- Workflow wall-clock fits within one prod deploy cycle (~1 week) by default. Longer wall-clocks require an entry in `docs/temporal/long-running.md` with a versioning plan and replay tests.
- Workflow code is deterministic; side effects go through activities; the `workflowcheck` analyzer enforces this in CI.
- Activities are idempotent and accept retries.
- Workflow IDs encode business intent (`payment-{order_id}`), not opaque UUIDs.
- Activity inputs and outputs stay in kilobytes; larger payloads go through the off-cluster bucket via reference.
- Periodic business-meaningful work uses Temporal `Schedule`. k8s `CronJob` is reserved for pure infrastructure tasks.
- Temporal Cloud is not used. The self-hosted server is the platform's runtime.
