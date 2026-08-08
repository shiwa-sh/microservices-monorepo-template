# ADR-0302: Durable Execution (Temporal)

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0100](0100-language-and-runtime.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0205](0205-environment-parity.md), [ADR-0300](0300-data.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0500](0500-observability.md)

## Context

The platform needs one answer to a family of related problems:

- Operations spanning **multiple services or external systems** that must not leave the system half-applied.
- Operations needing **compensation on failure**.
- **Scheduled and periodic work** — reports, reconciliations, cleanup.
- **Background and async work** — emails, thumbnails, indexing, fan-out.
- **Authz-relevant mutations** ([ADR-0304](0304-identity-and-authorization.md)) writing to both the application database and the authorization datastore, atomically in effect.

Without one durable-execution platform, each grows its own machinery: outbox tables, dead-letter queues, retry loops, cron jobs, custom idempotency. Repeated across a fleet, that is unmaintainable.

## Decision drivers

1. **One reliability primitive, not five.** No coexistence of DLQ, cron, and ad-hoc retries with a workflow engine. One sanctioned lighter path is the single deliberate exception, bounded below.
2. **Service boundaries stay HTTP and OpenAPI.** A workflow engine must not become the cross-service bus.
3. **Operationally cheap per workflow.** Adding one must cost less than building the same reliability by hand.
4. **Self-host** ([ADR-0000](0000-platform-foundations.md), principle 3).

## Considered options

| Option | Model | Maturity | Verdict |
| --- | --- | --- | --- |
| **Temporal, self-hosted** | durable execution as a first-class primitive: activities, child workflows, signals, queries, timers, schedules, sagas in one model | mature Go SDK, Postgres-backed | **Chosen** |
| Restate | similar durable-execution model with a simpler operational shape | young; SDK and saga ergonomics trail | Worth watching, not adopting |
| Cadence | Temporal's predecessor | less momentum | No upside over Temporal |
| DIY — outbox, queue, cron, retry loops | four mechanisms | ours to maintain | Re-implements Temporal poorly, four times |
| Kubernetes Jobs, CronJobs, Argo Workflows | pipeline DAGs | mature | Not business workflows. Retained for pure infrastructure tasks |
| Temporal Cloud | managed | mature | Excluded by driver 4. Client call sites stay cloud-neutral, so the position is reversible by deploy |

## Decision

Self-hosted Temporal is the platform's durable-execution layer and the default async primitive.

### When something is a workflow

A workflow is the right primitive when **any** of these hold:

1. The operation has two or more logical steps where partial completion is a bad state.
2. It touches more than one service or external system.
3. It must compensate on failure rather than return an error.
4. It is long-lived relative to a request.
5. It must survive process restarts by design.

It is the wrong primitive for a single atomic action inside one service.

| Operation | Primitive |
| --- | --- |
| Register user — identity, orgs, authz tuples, welcome email | Workflow |
| Checkout — reserve, charge, order, deduct, confirm | Workflow |
| Payment, often a child workflow of checkout | Workflow |
| Refund — compensation across systems | Workflow |
| Authz-relevant resource mutation | Workflow ([ADR-0304](0304-identity-and-authorization.md)) |
| Daily reconciliation | Workflow on a Temporal `Schedule`, not a `CronJob` |
| Transactional email, fire-and-forget | Outbox, or a workflow if delivery must be tracked |
| Thumbnail or document index | Outbox, or a workflow if part of a larger process |
| Update profile name, add to cart, list orders | Neither — synchronous |

### The outbox seam

A **trivial best-effort** job may use a transactional outbox instead: the triggering row and an `outbox` row commit in one Postgres transaction, and a small per-service dispatcher drains it.

A job stays on the outbox only while **all four** hold. The moment one fails, it is a workflow:

1. one logical step,
2. inside one service, with no cross-service coordination that must not half-apply,
3. best-effort — losing or double-running it is acceptable, so at-least-once dispatch with an idempotent handler suffices,
4. no compensation on failure.

Two systems exercise Temporal so far, which is not enough usage to harden a blanket "all async is a workflow" rule. Temporal is the default; the outbox is the sanctioned lighter path for the trivial case; the blanket rule is revisited once more workflows exist.

### Architecture: co-located workflows, HTTP between services

Workflows, activities, and workers live inside the service that owns the business process.

```text
services/<service>/
├── openapi.yaml
├── cmd/{server,worker}/main.go
├── internal/
│   ├── handlers/        # HTTP handlers from generated server stubs
│   ├── workflows/       # workflows owned by this service
│   ├── activities/      # activities owned by this service
│   ├── domain/
│   └── store/
└── migrations/
```

**Process-owner rule.** A workflow lives in the service owning the *business process*, not the one owning the most data. Register-user lives in identity even though it writes to orgs and the authz store; checkout lives in checkout even though it calls payment, inventory, and orders. A service whose primary job is orchestrating others is a legitimate shape.

**Cross-service invocation is HTTP only:**

1. The owning service exposes an HTTP endpoint that internally starts the workflow.
2. The caller invokes it through the generated client.
3. The response is `202 Accepted` with a handle conforming to the `WorkflowHandle` schema ([ADR-0303](0303-api-contracts-and-lifecycle.md)).

A service never starts another service's workflow through the Temporal client. Doing so imports the callee's workflow input struct and bypasses OpenAPI, tracing, and the identity-header contract.

**Waiting on a cross-service workflow**, chosen by need:

| Mechanism | How |
| --- | --- |
| Poll the handle | the owning service exposes `GET /<resource>/{id}` returning `{status, result?}`; the caller's workflow polls with backoff |
| Webhook callback | the caller passes `callback_url`, the owning service POSTs on completion, and the caller's workflow waits on a signal raised by its own webhook handler |
| Fire-and-forget | the caller does not need the result |

Direct Temporal signals across service boundaries are not used.

### Activity placement

| Scope of use | Location | Note |
| --- | --- | --- |
| One service's workflows | `services/<service>/internal/activities/` | the common case |
| Generic infrastructure — email, object storage, metrics, webhooks | `libs/go/temporal-activities/<concern>/` | stateless and service-agnostic, with no dependency on `services/` |
| Logically owned by another service | **not shared** — each caller writes a thin activity wrapping the owning service's generated client | the shared thing is the HTTP API |

Sharing an activity across services by putting domain logic in `libs/` couples them behind the API boundary [ADR-0000](0000-platform-foundations.md) principle 10 establishes.

### Wall-clock

A workflow's wall-clock fits inside one production deploy cycle by default. The reason is event-history size and operational legibility, not avoiding versioning, which the next section handles directly.

A longer wall-clock is permitted and requires:

1. an entry in `docs/temporal/long-running.md` with the expected wall-clock,
2. replay tests over historical event histories in CI,
3. a documented `workflow.GetVersion` patching plan **if and only if** the workflow opts out of Pinned into `AutoUpgrade`, which is the one case that moves between versions mid-flight.

### Worker Deployment Versioning

The failure mode: a workflow started before a deploy is resumed by a worker running after it, and if the code no longer replays the recorded history the execution dies with a non-determinism error. A short wall-clock shrinks the exposure window without closing it — a thirty-second checkout can still be mid-flight when a worker rolls.

**Workers run versioned, and workflows are Pinned by default.** Each worker declares a Worker Deployment Version — a deployment name plus a Build ID. The server routes new executions to whichever version is Current, and a Pinned execution runs start to finish on the version that began it. A deploy therefore cannot break in-flight work, and the ordinary case needs no `workflow.GetVersion`. Temporal's own guidance makes versioning, not patching, the production default.

Four consequences, each of which has produced an incident:

| Consequence | Detail |
| --- | --- |
| **The Build ID tracks the code, not the release channel** | The controller derives it from the image tag plus a hash of the pod template, so a configuration-only change is also a new version. Stricter than deriving from the image alone ([ADR-0103](0103-release-and-versioning.md)), which is why the chart does not compute it |
| **A pin is only as good as the pods behind it** | Pinning to a version that can no longer be reached is worse than not pinning: the execution does not fail over, it waits. A rolling `Deployment` deletes the old version's pods the instant rollout completes, so protection would last seconds rather than the workflow's lifetime. Workers are therefore `WorkerDeployment` CRs reconciled by the **Temporal Worker Controller**, which keeps one Deployment per Build ID alive until Temporal reports it Drained. Retaining a version until its work finishes depends on drainage state only Temporal knows, changing long after a sync ends, so it is reconciler work rather than something a chart can template |
| **Promotion belongs to the controller** | It registers each version, promotes per `rollout.strategy`, and injects the deployment and build-ID environment variables itself. Upstream is explicit that those must not be set by hand. Rollback sets the current version to the previous Build ID, whose pods still exist precisely because sunset is delayed |
| **Sessions and versioning are mutually exclusive** | The SDK refuses `EnableSessionWorker` together with versioning, so a service needing sessions opts out explicitly and owes a patching plan instead |

**Deleting a `WorkerDeployment` blocks while its workers still poll.** The CR carries a delete-protection finalizer; on delete the controller asks Temporal to remove each version, and Temporal refuses while pods of that version exist and for a further poller-expiry window after they are gone. The controller retries indefinitely, so the CR sits `Terminating` and Argo cannot recreate it. Removing a worker therefore means scaling the versioned Deployments to zero first and waiting for pollers to expire. The break-glass is patching the finalizer away, which leaves the Temporal-side version records to age out.

### Draining a worker on rollout

Distinct from replay safety and routinely confused with it: when a worker pod is rolled, **activities currently executing are lost unless the worker is told to wait**. The Go SDK's `WorkerStopTimeout` defaults to no wait, so the platform sets it explicitly, and the chart derives `terminationGracePeriodSeconds` from it so kubelet always waits strictly longer than the worker. Configuring the grace period independently is how this silently regresses.

### What Temporal replaces

| Legacy pattern | Replacement |
| --- | --- |
| Multi-step or cross-service outbox machinery | Workflows. The database write and the downstream effect are activities in one workflow. The transactional outbox survives only for the trivial best-effort case |
| Event bus (NATS, Kafka) | **Not adopted.** HTTP plus signals via webhook callbacks cover cross-service notification. A true pub/sub fan-out need gets its own ADR |
| Kubernetes `CronJob` for business-meaningful work | Temporal `Schedule`. `CronJob` remains for pure infrastructure |
| Background queues | Workflows on a `background-tq` queue, except trivial best-effort jobs on the outbox seam |

### Operational shape

| Element | Value |
| --- | --- |
| Server | the Temporal monolith binary, all four roles in one process, via Helm, backed by a separate database on the platform Postgres cluster |
| Workers | one deployment per service, registering only that service's workflows and activities, shipping with its service |
| Task queues | named per service, plus a shared `background-tq` |
| Namespaces | one per environment |
| History retention | 30 days production, 7 days non-prod |
| Local | the inner loop uses `temporal server start-dev` for speed; the full tier runs this same chart at one replica, CNPG-backed ([ADR-0600](0600-local-development-loop.md)) |

### Conventions

| Convention | Rule |
| --- | --- |
| Determinism | no `time.Now()`, no `math/rand`, no direct I/O, no goroutines — use `workflow.Go`. Side effects go through activities |
| Idempotency | every activity tolerates being invoked twice with the same input. Run ID plus activity ID is the natural key for external calls |
| Payload size | activity inputs and outputs stay in kilobytes. Large payloads go through the object bucket, and activities pass references |
| Timeouts | explicit. Default activity `StartToCloseTimeout` 30s, default `WorkflowExecutionTimeout` 1h. Overrides carry a justification in the workflow file |
| Errors | typed through `temporal.NewApplicationError` with stable types, and retry policy keys off them |
| Workflow IDs | encode business intent — `payment-{order_id}`, not `payment-{uuid}` — because idempotency is a property of the business operation |
| SDK call sites | only in `internal/workflows/` and `internal/activities/` |

### Cross-cutting integrations

Authz dual-write discipline is [ADR-0304](0304-identity-and-authorization.md)'s, the `WorkflowHandle` shape is [ADR-0303](0303-api-contracts-and-lifecycle.md)'s, service-to-service identity propagation is [ADR-0304](0304-identity-and-authorization.md)'s, and trace propagation is [ADR-0500](0500-observability.md)'s. Nothing here overrides them.

## Consequences

### Positive

- One reliability primitive answers four problems, learned once, with no second runtime to operate.
- Saga compensation becomes routine rather than bespoke.
- Hand-rolled multi-step outbox machinery disappears; the outbox survives only as the deliberate lighter path.
- Authz dual-write risk is structurally solved rather than policed.
- Service boundaries remain HTTP and OpenAPI.

### Negative / Risks

- **Non-determinism rules are a real cognitive tax.** Mitigated by `workflowcheck`, a review checklist, and — for deploying under running executions — by Pinned versioning, which removes the need to reason about replay compatibility at all.
- **Versioning adds an operator to the platform surface**, with a webhook in the admission path and a reconciler that can fall behind. A stalled controller means new versions are never promoted and new executions have nowhere to run. It is a dependency of the deploy path, not a background convenience.
- **Old versions linger while their Pinned executions drain**, so a service runs more than one worker version at once for as long as sunset allows. Pod count and Temporal's version list exceed the naive desired state, and **a version whose workflows never finish never drains** — a long-lived workflow can pin one indefinitely. That is the real cost of the wall-clock rule being a rule.
- **The Temporal server is critical infrastructure.** Mitigated by HA Postgres and replay tests proving workflows tolerate restarts.
- **Per-activity latency in the tens of milliseconds** rules workflows out of sub-100ms request paths. The scope rule already excludes those.
- **One worker deployment per service multiplies pod count.** Accepted; it preserves ownership.

### Follow-ups

- `infra/helm/platform/temporal/` with Postgres backing.
- `libs/go/temporalmw/` for shared client config, default retry policies, tracing middleware, and replay-test scaffolding.
- `dep:temporal` bringing up Temporal when a service declares it.
- `golangci-lint` configured with `workflowcheck`.
- `docs/temporal/long-running.md`, initially empty.
- The `WorkflowHandle` schema declared inline in each service's spec.

## Rules

- Temporal is the platform's durable-execution mechanism and the default async primitive. No DLQs, ad-hoc retry loops, or cron jobs for business-meaningful periodic work. `(review-only)`
- A workflow exists if the operation matches at least one of the five scope criteria. A trivial best-effort job matching none may use the outbox seam. `(review-only)`
- Workflows, activities, and the worker for a service live under `services/<service>/`. There is no top-level workflow directory. `(CI: ci-lint)`
- Cross-service workflow invocation is HTTP through the generated client. Direct Temporal-client calls across service boundaries are not used. `(CI: ci-lint)`
- Cross-service result waiting is polling, a webhook callback, or fire-and-forget. Direct cross-service signals are not used. `(review-only)`
- Activities are placed by ownership and are never shared across services as a domain wrapper. `(review-only)`
- A workflow's wall-clock fits one production deploy cycle. A longer one requires an entry in `docs/temporal/long-running.md` with replay tests. `(review-only)`
- Workers run with versioning enabled and a default behaviour of Pinned. Disabling it is permitted only where the SDK forbids the combination, and obliges that service to a documented patching plan. `(review-only)`
- A versioned worker is a `WorkerDeployment` reconciled by the controller, never a plain `Deployment`. `(review-only)`
- Build IDs and the deployment and build-ID environment variables are set by the controller alone. `(review-only)`
- Sunset delays are only lengthened, never shortened toward zero. `(review-only)`
- A workflow opting into `AutoUpgrade` owes a `workflow.GetVersion` patching plan and replay tests. `(review-only)`
- Worker graceful-stop is configured, and `terminationGracePeriodSeconds` is derived from the worker stop timeout rather than set independently. `(review-only)`
- Workflow code is deterministic and side effects go through activities. `(CI: ci-lint)`
- Activities are idempotent and accept retries. `(review-only)`
- Workflow IDs encode business intent, not opaque UUIDs. `(review-only)`
- Activity inputs and outputs stay in kilobytes; larger payloads go through the bucket by reference. `(review-only)`
- Periodic business-meaningful work uses a Temporal `Schedule`. `CronJob` is reserved for pure infrastructure. `(review-only)`
- Temporal Cloud is not used. `(review-only)`
