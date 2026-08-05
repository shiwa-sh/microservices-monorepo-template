# ADR-0016: Environment Parity & Local Tiers

- **Status:** Accepted
- **Date:** 2026-07-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0003](0003-cluster-topology.md), [ADR-0004](0004-gitops.md), [ADR-0005](0005-secrets.md), [ADR-0007](0007-data.md), [ADR-0011](0011-observability.md)

## Context

[ADR-0003](0003-cluster-topology.md) commits to parity at the artifact layer: production is **k3s on plain compute**,
local is **k3d (k3s-in-Docker)**, and the same Helm charts and manifests apply. That leaves three questions this ADR
answers precisely:

- **What may differ** between an engineer's laptop and production, and what may never differ.
- **How the local cluster is composed** when different roles need different subsets of the platform (a frontend
  engineer iterating on RUM dashboards, a backend engineer debugging one service, an operator running a full
  end-to-end test).
- **Where GitOps fits locally**, given ArgoCD reconciles committed git state, not a working tree.

## Decision drivers

1. **Parity is at the artifact layer.** Same charts, same images, same code, same commands. Differences are declared as
   per-env values, never as forked manifests or chart logic branching on environment.
2. **Two different local jobs.** "Iterate on my code fast" and "validate the whole platform behaves like prod" are
   distinct goals with opposite optimisation targets (speed vs fidelity). One local tier cannot serve both well.
3. **Uphold existing ADRs.** [ADR-0003](0003-cluster-topology.md) (external bucket + off-cluster backups in prod) and
   [ADR-0005](0005-secrets.md) (one secret mechanism everywhere) stand unchanged.
4. **Composition over forks.** A role-specific local, or a new environment, is a thin values selection — not a new
   script or a copied manifest set.

## Decisions

### Parity model: same charts, per-env values only

Every environment — **local, dev, staging, prod** — deploys the same charts under `infra/helm/platform/*` and
`infra/helm/service`. The only sanctioned divergence is a per-env values overlay (`infra/gitops/platform/<env>/values.yaml`,
with local as a first-class member). A divergence that cannot be expressed as a value — a different chart, or a
hand-written Deployment standing in for an operator-managed component — is permitted **only** in the inner-loop tier
below, and never silently.

The **contract never differs in any tier**: the Kubernetes API, the service chart, the service images, and the env
contract (`DATABASE_URL`, `TEMPORAL_HOST_PORT`, OTLP endpoint, OpenFGA) are identical everywhere. What may differ is
*scale* (replica counts, storage), and — in the inner loop only — the *implementation behind a contract*.

### The approach: local-first, and when that stops being true

Local Kubernetes development has four established approaches. We evaluated all four rather than defaulting to one,
because the right answer is a function of service count and changes as a project grows:

| Approach | How it works | Pros | Cons | Verdict |
|---|---|---|---|---|
| **Local cluster** | Scaled-down whole system on the laptop (kind/k3d/k3s) | No shared infra, no platform team, full isolation, works offline | Laptop resource ceiling; breaks down past ~20 services; stand-ins can drift from real deps | ✅ **Chosen** |
| **Sync tools** | A tool watches files, rebuilds images, redeploys into a cluster | Fast container loop; one tool owns build→deploy→watch | A second orchestrator beside Helm/Argo; still an image build on the hot path | ➖ Partially — the deploy step, not the watch loop |
| **Shared cluster, one service local** | The base cluster runs remotely and shared; your service runs locally and joins it | Highest fidelity; real deps at current versions; no mock drift; cheap per engineer | Needs a platform team; contention unless request-level isolation is built; requires trace-context propagation | ❌ Not at this size |
| **Cloud workspaces** | The whole dev environment moves to a remote machine | Uniform environment; no laptop limits | Cost per engineer; network-dependent; weakest local tooling | ❌ Orthogonal to parity |

**The ~20-service ceiling is the trigger to revisit, and it is a real number, not a hedge.** "Run it all locally" is
the consensus-correct choice below it and the consensus-wrong one above it. This repo has five services and a
frontend. A project built on this template that grows past the ceiling should expect to move to the third approach —
and the cheap thing to protect in the meantime is **trace-context propagation**, because request-level isolation
(Lyft's staging overrides, Uber's SLATE, Signadot's sandboxes) is built on it. OTel is already wired up; keeping
propagation honest is what keeps that door open.

Sync tools are rejected for a reason specific to this design, worth stating because it is counter-intuitive: **our
inner loop runs the service natively on the host, so there is no container to sync into.** File sync is the headline
feature of Skaffold, Tilt, DevSpace and Okteto, and it solves a problem we do not have. See
[ADR-0030](0030-dev-loop-tooling.md).

### Two local tiers

Local is two tiers with different jobs:

| Tier | Command | Parity | What runs | For |
|------|---------|--------|-----------|-----|
| **Inner loop** | `cluster:base` + a service's own tasks | **interface** | k3d + the local floor; the service under change runs natively on the host | day-to-day coding |
| **Full platform** | `cluster:full` | **implementation** | k3d + the real platform charts at `instances=1` (CNPG, the Temporal chart, OpenFGA, MinIO, the observability stack, the edge + auth) | end-to-end tests ([ADR-0018](0018-testing-strategy.md)), pre-merge validation, CI, label-gated per-PR preview |

The **inner loop** optimises for speed. The service under change runs **natively on the host** (any editor/IDE, or
`go run`) against lightweight stand-ins (a plain Postgres, `temporal server start-dev`, in-memory OpenFGA; see
[ADR-0003](0003-cluster-topology.md), [ADR-0006](0006-temporal.md)) reached through `dev:forward` port-forwards. The
stand-ins are acceptable because they honour the same wire contract — a bug reproduced against them reproduces in prod.
There is no image build, in-cluster redeploy, or file-watch on the hot path; the full platform is not running, so the
loop is fast and light.

Within the inner-loop tier a service is in one of two states, and switching is one command either way:

| Mode | What runs | Cost | For | Limits |
|---|---|---|---|---|
| **Native** (`service:dev` + `mise run server`) | the service on the host, behind the real edge | lightest | day-to-day coding | no pod env/files; no NetworkPolicy enforcement |
| **In-cluster** (`cluster:add`) | the service from the working tree, in the cluster | image build per change | "I need it running, I'm not editing it" | rebuild on every change; no debugger attach |

The two are per-service, not per-session: **any number of services can be native at once**, each bound to its own
registered port. Debugging a flow that spans three services means running all three natively, with breakpoints in all
three, while everything they call stays in the cluster.

That requires a **committed port registry** (`scripts/lib/ports.sh`), not a convention. Every server binds `:8080`
in-cluster, which collides the moment a second one runs on the host — and ad-hoc ports would live in each engineer's
gitignored `.env`, where they cannot be shared or defaulted. One stable port per service, identical on every machine,
is what lets a caller's `CATALOG_URL` ship a *working* default. `mise run lint:ports` enforces uniqueness and that each
service binds what the registry assigns it; `:8080` stays unassigned because k3d maps host `8080` to the edge.
In-cluster is unaffected — the chart sets no `PORT`.

The **full platform** optimises for fidelity. It runs the **same charts production runs**, scaled to a single replica
through the `local` values overlay — CNPG (not a plain Postgres pod), the Temporal Helm chart (not `start-dev`), the
OpenFGA chart, in-cluster MinIO, and the observability stack. This is the tier that catches operator behaviour, sync
ordering, and chart wiring before code reaches a deployed environment, and it is the exact configuration CI and per-PR
preview environments use. It is also the environment the end-to-end and visual suites run against ([ADR-0018](0018-testing-strategy.md)):
the full e2e suite nightly + pre-release, a smoke subset on label-gated per-PR previews.

### Distribution tiers: k3d (ephemeral) vs k3s (persistent)

`k3d` and `k3s` run identical charts, so the choice is lifecycle, not architecture:

| Tier | Distribution | Lifecycle |
|------|--------------|-----------|
| local (both loops) | k3d | per-engineer, recreated freely |
| CI e2e / label-gated per-PR preview | k3d | ephemeral, created and destroyed per run |
| dev / staging / prod | k3s on compute | persistent ([ADR-0003](0003-cluster-topology.md)) |

The full-platform local tier and the CI/preview tier are the **same configuration**; investment in one is investment in
the other. Persistent dev/staging stay on k3s-on-compute — they survive reboots and hold real storage and backups, and
they are not managed Kubernetes, so there is no control-plane bill to cut by moving them to k3d.

### Local composition: a floor, plus per-service dependency declarations

There are no named profiles. An earlier revision of this ADR declared five (`min`, `backend`, `edge`, `obs`, `full`);
two were never built, and the ones that were turned out to be hand-built tiers rather than compositions. The
replacement is smaller and cannot drift:

**A floor.** `cluster:base` brings up Traefik, cert-manager, Postgres, Kratos and Oathkeeper, plus the host edge glue
and the seeded test identities. It is unconditional — the Docker Compose profiles convention (base services always
start, optional ones are tagged) applied at the cluster level.

The identity stack is in the floor deliberately, generalising [ADR-0029](0029-api-mocking-and-ui-dev-loop.md)'s
argument from the frontend to every service: Kratos and Oathkeeper are cheap, and their behaviour — cookies, CSRF,
session expiry, AAL, `401`/`403` — **is** the contract every service consumes. A service reached through this edge
gets real Oathkeeper identity headers; one curled directly on `:8080` is being handed forged ones. The cost is that a
backend engineer who only wants Postgres still pays for the edge; that trade is accepted here rather than left to be
discovered.

**Everything else is opt-in, declared by the service that needs it.** Each service names what it needs in its own
`.mise.toml`, in two families, and mise resolves the graph — deduping the shared `cluster:base` edge, running
independent edges in parallel:

| Family | Declares | Satisfied by |
|---|---|---|
| `dep:*` | **Infrastructure** the service reads — Postgres, Temporal, OpenFGA | A label slice of `infra/local/deps.yaml` |
| `svc:*` | **Other services** it calls over HTTP | The callee deployed and forwarded to its registered local port |

So **what is up is base ∪ the declared dependencies of whatever you are running.** There is no table to keep honest,
because the service knows its own dependencies and a named profile never can.

`svc:*` exists because the second family is the one a microservices repo cannot omit. The orders checkout saga calls
catalog and payment ([ADR-0006](0006-temporal.md)), so a worker without them starts cleanly and dies on the first
checkout — the failure is *later* than a missing database and therefore harder to attribute. An earlier revision
covered `dep:*` only and left callees to a comment in `.env.example`; that comment asked the engineer to know the
callee list, deploy each one, forward each one, and edit `.env`, which is precisely the scavenger hunt
[ADR-0030](0030-dev-loop-tooling.md) rejected Tilt for.

**Deploying the callee is the default, and running it natively is the override.** The usual case is working on the
caller, where callees should be opaque. So `svc:catalog` guards on *the port being answered*, not on a deployment
existing: start catalog natively yourself and the graph sees it answered and leaves it alone. That is what makes the
two scenarios one mechanism — the cross-service debugging case is the single-service case with more of the callees
run by hand.

### The service contract

The model above only works if every service participates in it the same way, so this is the standard a new service is
held to. It is short by design, and it is checked: `mise run lint:service-contract`.

| A service must | Because |
|---|---|
| Live at `services/<name>/`, name matching everywhere | `cluster:add` resolves names over services and platform charts as two disjoint namespaces |
| Expose the standard tasks — `server`, `test`, `lint`, `build` (+ `worker`, `migrate`, `generate` if it has them) | [ADR-0002](0002-monorepo.md); CI and `cluster:add` invoke them by name |
| Register a local port in `scripts/lib/ports.sh` and set the same `PORT` in its `.mise.toml` | So it can run natively alongside others, and so callers' `<SVC>_URL` can have a working default |
| Declare `dep:*` for the infrastructure it reads | The graph brings up exactly that, and the deploy path reads the same list |
| Declare `svc:*` for every service it calls over HTTP | Otherwise it starts cleanly and fails on the first cross-service call |
| Ship `.env.example` covering every variable it reads | `mise run server` seeds `.env` from it on a fresh clone; an unlisted variable is a silent default |
| Ship a values file per environment under `infra/gitops/services/<env>/values/`, or declare `# platform/not-deployed: <env>` | **The ApplicationSet generates one Argo Application per values file.** No file means the service is absent from that environment, and nothing reports it |
| Ship a `Dockerfile` and a `README.md` | The image is built by the same path in CI and `cluster:add`; the README is where the service's own quirks go |
| Bind `httpmw.ListenAddr()` | `:8080` in-cluster — the chart's containerPort, the edge IngressRoutes and the NetworkPolicies all assume it |

**The values-file rule is the one that has already bitten.** `infra/auth/oathkeeper/values.yaml` points the
`remote_json` authorizer at `authz-server` by service DNS in *every* environment, but `authz` had a `local` values file
and no `dev` one — so it was never deployed to dev, and every gated dashboard request there resolved to a Service that
did not exist. Nothing failed loudly, because absence is not an error state in a git-directory generator. That is why
the contract requires an explicit opt-out rather than accepting silence: the deployment surface of this platform is the
set of values files, and it must be stated, not inferred.

**What the check cannot do is verify the declarations are true.** That a service listing `dep:temporal` really uses
Temporal, or that a new HTTP call gained its `svc:*` edge, is a reviewer's job — the same way the linter can see that
an `.env.example` exists but not that it is complete. Adding a cross-service call without its `svc:*` edge is the most
likely way to regress the local loop.

A component is still a **selection, not a copy**: every one is the same chart with the same
`infra/gitops/platform/local/values.yaml` overlay the full tier uses, and `infra/local/deps.yaml` is sliced by the
`local.platform/component` label rather than forked. One local values file; nothing can drift from the tier above it.

**Which service I iterate on** is the second axis and never leaks into composition. In the inner loop you run one
service natively; the rest are absent or in-cluster. `cluster:add -- <name>` does a one-shot build-import-upgrade for
a service, or a working-tree overlay for a platform chart ([ADR-0003](0003-cluster-topology.md)); `cluster:remove`
reverses it *and restores ArgoCD auto-sync*, which the add path paused.

Dependency components are deliberately **not** addressable through `cluster:add`. `postgres` is both a platform chart
and a `deps.yaml` slice, so making components user-facing nouns would be ambiguous on day one. They stay
graph-internal, which keeps `cluster:add` resolving over two disjoint namespaces — services and platform charts — and
a name appearing in both is treated as a repo bug, not a user error.

ArgoCD is the engine for `full` only; the floor and the components are applied imperatively for the same reason the
inner loop is (see below), which is also why base runs the ephemeral Postgres stand-in rather than CNPG.

### GitOps locally: inner loop is native, full tier is ArgoCD

ArgoCD reconciles committed git state, so it is **not** the inner loop's engine — the inner loop runs the service
natively against `cluster:base`'s stand-ins ([ADR-0004](0004-gitops.md)). The **full-platform tier does run ArgoCD**: a
local bootstrap (`infra/gitops/local-bootstrap/`) applies the same app-of-apps prod uses, syncing committed `master`
from the remote, so sync ordering, app discovery, and secret materialisation are exercised exactly as in prod. Only the
two genuine bootstrap components ArgoCD cannot self-create — the CNI (Cilium) and ArgoCD itself — are installed
imperatively by `cluster:full` before the root-app is applied; everything else is Argo-managed.

Two escape hatches cover iterating on uncommitted infra (a rare day):

- **Chart/value change:** `platform:deploy -- <chart>` pauses Argo auto-sync on that one app and `helm upgrade`s it from
  the working tree.
- **GitOps-wiring change** (sync-waves, ApplicationSets, App defs): push a branch and point the local root-app
  `targetRevision` at it — this exercises the real delivery path, which `helm` cannot.
- **CNI/CRD change** (e.g. Cilium): prefer `cluster:delete` + a fresh `cluster:full` over an in-place upgrade — hot-
  swapping a CNI on a live cluster blips networking. This is inherent to the component, not a tooling gap.

This is what makes the full tier a true e2e rehearsal rather than a hand-rolled approximation.

### Object storage: MinIO (S3 API) in non-prod, external bucket in prod

[ADR-0003](0003-cluster-topology.md) stands: **no MinIO in production**, **backups off-cluster and mandatory**. The
interface is the S3 API in every environment; the provider differs by values:

- **local / dev / staging:** in-cluster MinIO (`infra/helm/platform/minio`).
- **prod:** an external S3-compatible bucket. In-cluster object storage in prod would co-locate data and its backups on
  the same nodes, defeating disaster recovery.

CNPG backups target the external bucket in prod; in non-prod they may target the in-cluster MinIO for self-containment
(non-prod backups are convenience, not a recovery guarantee). The only per-env delta is the S3 endpoint and whether the
MinIO chart is enabled.

### Secrets: SOPS everywhere

[ADR-0005](0005-secrets.md) requires one secret mechanism in every environment. Local is no exception: the same SOPS
path, with a committed, well-known **local** age key — safe precisely because the only values it decrypts are throwaway
dev credentials. The decrypt mechanism is identical to prod; only the plaintext differs.

### Local domain: `*.localtest.me`

`localtest.me` is real public DNS resolving to `127.0.0.1` with zero host configuration. `.local` is reserved for mDNS
([RFC 6762](https://www.rfc-editor.org/rfc/rfc6762)) — it collides with Avahi/Bonjour and does not resolve to loopback
without `/etc/hosts` or mDNS. `*.localhost` / `*.test` signal "local" more loudly but need a modern resolver or a hosts
entry. The frictionless default wins; the `dev.` prefix already marks it non-production.

### What is local-only by nature

A small, enumerated set of manifests has no production analogue, and each states why:

- `infra/local/deps.yaml` — the inner-loop dependency stand-ins. The full tier does not use it.
- `infra/local/traefik-config.yaml` — tunes the k3s-bundled Traefik (cross-namespace refs). Prod runs its own Traefik
  (the Ansible `k3s_server` role disables the bundled one).
- `infra/local/edge-auth.yaml` — routes `/auth` + landing to a host-run `next dev`, a dev-loop convenience
  ([ADR-0014](0014-frontend.md)); prod deploys the built frontend image in-cluster.
- `infra/local/mock.yaml` — the API mock serving the committed OpenAPI projection on `/api`
  ([ADR-0029](0029-api-mocking-and-ui-dev-loop.md)). Added on top of the floor when you want it; it
  runs in no deployed environment.
- `infra/local/coredns-rewrite.yaml` — resolves the env host to Traefik from inside the cluster.
  `dev.localtest.me` is real public DNS pointing at `127.0.0.1`, which in a pod is the pod's own
  loopback, so an in-cluster frontend would otherwise dial itself.
- A local CA issuing the wildcard instead of cert-manager + Let's Encrypt — the same cert-manager
  mechanism, a local ClusterIssuer. It is a **CA**, not a self-signed leaf: a leaf cannot be a trust
  anchor, which forced `NODE_TLS_REJECT_UNAUTHORIZED=0` and blocked the frontend from ever running
  as its production image locally.

## "May differ" vs "must not differ"

| Concern | May differ across envs | Must NOT differ |
|---------|------------------------|-----------------|
| Kubernetes distribution | k3d (ephemeral) vs k3s (persistent) | the charts and manifests applied |
| Scale | replicas, storage size, anti-affinity, HPA | which components the full tier runs |
| Data-tier implementation | inner-loop stand-ins vs the real charts | the wire contract (`DATABASE_URL`, Temporal gRPC, OpenFGA), the Postgres major |
| Object storage provider | MinIO (non-prod) vs external bucket (prod) | the S3 API contract |
| TLS issuer | local CA (local) vs Let's Encrypt (deployed) | cert-manager as the mechanism; that certs are **verified**, never bypassed |
| Secret plaintext | throwaway (local) vs real | SOPS as the decrypt mechanism |
| Domain | `*.localtest.me` vs `*.<env>.<project-domain>` | the routing / edge shape |

## Consequences

- **The full-platform local tier validates the same software prod runs** — operators, sync ordering, and chart wiring,
  not just the services. A chart change is exercised end-to-end before it reaches a deployed environment.
- **The inner loop stays fast.** Engineers who only need to run code keep the lightweight stand-ins; the heavy tier is
  opt-in via `cluster:full`, and above the floor you pay only for what your service declares.
- **Re-entering the graph must stay cheap.** mise resolves dependencies but has no cluster-state awareness — its
  `sources`/`outputs` staleness is keyed on files, and "is Temporal already Ready?" is not a file. Every component
  installer therefore fast-exits when already up, centralised in `scripts/dep-apply.sh` so no component can forget.
  Without that guard every service run re-pays a full bring-up and this model is slower than the one it replaced.
- **`svc:*` introduces the one long-lived process the dev loop owns.** A mise task must return for the graph to
  continue, but the callee's port-forward has to outlive it, so `scripts/svc-apply.sh` detaches one and tracks it by
  pidfile. It is reaped by `cluster:remove` and re-created on demand when a stale one is found. This is the least
  elegant part of the design and the price of not adopting a tool that manages processes for us; it is confined to one
  script deliberately. If it becomes a source of bugs, `mirrord` ([ADR-0030](0030-dev-loop-tooling.md)) subsumes it.
- **A service's callee list is now load-bearing.** Adding a cross-service HTTP call means adding a `svc:*` edge, or the
  native loop regresses to the failure this closed. Nothing detects an omitted edge — the call simply fails at runtime,
  as it did before.
- **`infra/local/` is a short, justified list** — inner-loop deps plus the genuinely-local edge shims — not a parallel
  copy of the platform.
- **The full tier costs laptop RAM.** Running CNPG + the Temporal chart + the observability stack at once is heavier
  than the inner loop; per-service dependency declarations exist precisely so day-to-day work runs only the slice it
  needs.
- **The floor is not free.** A backend engineer who wants only Postgres still pays for Traefik, cert-manager, Kratos
  and Oathkeeper. That is the price of every service seeing real identity headers, and it is accepted knowingly.
- **New environments and role-locals are values selections.** Adding one is an overlay, not a script.
