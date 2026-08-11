# ADR-0600: Local Development Loop

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0100](0100-language-and-runtime.md), [ADR-0101](0101-monorepo.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0205](0205-environment-parity.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0400](0400-frontend.md), [ADR-0601](0601-testing-strategy.md)
- **Decides:** Two local tiers — `cluster:base` for the inner loop and `cluster:full` for the platform — with a vendored mock and no second definition of the system.

## Context

[ADR-0205](0205-environment-parity.md) fixes the parity contract across environments. This ADR settles what runs on an engineer's machine: which tiers exist, how a tier is composed, what builds it, and which parts of the platform are replaced by their contract.

Three jobs need different tiers, and one tier cannot serve all three:

| Job | Optimises for | Needs |
| --- | --- | --- |
| Iterate on one service's code | speed | that service natively, its dependencies reachable |
| Build a UI screen | speed, but across many services | an authenticated page; the services behind it are not under change |
| Validate the platform behaves like production | fidelity | the real charts, the real operators |

The second job has no cheap answer by default. A single panel screen fans out across `/products`, `/orders`, `/orgs`, and `/charges` — four services the engineer is not changing.

The full-platform tier serves them at a footprint the platform dominates entirely, the observability stack outweighing every service ([ADR-0601](0601-testing-strategy.md)). None of that renders a table, and the gap widens as the fleet grows: services behind a screen grow while services under change stay at one.

Two properties make the missing tier cheap:

- [ADR-0303](0303-api-contracts-and-lifecycle.md) emits exactly the artifact a mock needs. The `gen:openapi-public` projection at `apps/frontend/public/devportal/openapi/internal.json` is one merged OpenAPI 3.1 document holding precisely the operations at audience `>= internal`, `servers: /api`, committed and drift-checked.
- The auth stack is small. Kratos plus Oathkeeper plus an ephemeral Postgres is a rounding error against the full tier.

The question is therefore not how to simulate the platform, but **which parts are cheap enough to keep real, and which are worth replacing with their contract**.

## Decision drivers

1. **The spec is the only source of mock behaviour.** A response shape not derivable from a committed OpenAPI artifact is not served.
2. **No development-only code in the application**, and never in the authentication path. A branch in the app's authn gate that grants a session is production risk taken for developer convenience.
3. **A tier costs what it runs.** The question for each component is whether keeping it real is cheaper than replacing it with its contract, so the design is subtractive.
4. **Nothing new in any deployed environment.** This is local tooling: no chart, no environment, no image built from our source.
5. **Coexistence with Helm and Argo CD.** A second orchestrator that wants to own the same resources is a conflict.
6. **A tool adopted here is inherited, not chosen.** Every derived project gets this toolchain without re-deciding it, so active maintenance and an OSS licence are entry gates rather than scoring columns. Popularity is not a fitness measure.

## Considered options

### Local Kubernetes approach

| Approach | How it works | Cost | Verdict |
| --- | --- | --- | --- |
| **Local cluster** | scaled-down whole system on the laptop | laptop resource ceiling; stand-ins can drift | **Chosen** — no shared infra, no platform team, full isolation, works offline *(reasoned)* |
| Sync tools | a tool watches files, rebuilds images, redeploys | a second orchestrator beside Helm and Argo; an image build on the hot path | **Partially** — the deploy step, not the watch loop |
| Shared cluster, one service local | the base cluster runs remotely; the service under development joins it | needs a platform team, and contention unless request-level isolation is built | Highest fidelity, and the destination past the ceiling below |
| Cloud workspaces | the whole dev environment moves to a remote machine | cost per engineer, network-dependent, weakest local tooling | Orthogonal to parity |

### Local Kubernetes distribution

The approach above says "a cluster on the laptop" and this row says which one. Every option runs the same charts ([ADR-0205](0205-environment-parity.md)), so the comparison is startup cost and how images get in.

| Option | Node shape | Getting an image in | Distance from [ADR-0200](0200-cluster-topology.md)'s production distribution | Verdict |
| --- | --- | --- | --- | --- |
| **k3d** | k3s in Docker containers | `k3d image import`, or the built-in registry | k3s, not Talos-shipped upstream Kubernetes | **Chosen.** The fastest create-and-destroy cycle of the field, and a built-in registry means the image path is one command rather than a local registry to run *(measured)* |
| kind | upstream Kubernetes in Docker | `kind load docker-image` | **upstream Kubernetes, as production runs** | The closest to production of the four, and slower to create and to load images into. Its advantage is the control plane the cluster runs, which matters for a Kubernetes-version-sensitive change and not for a service change |
| minikube | a VM or a container, many drivers | `minikube image load`, or its Docker daemon | upstream, with its own addon layer | The most portable across host operating systems, and the heaviest per cluster. Its addons are a second source of cluster configuration, against parity |
| Talos in Docker | **Talos, as production runs** | a local registry | **none — the same OS and distribution** | The highest fidelity available, and the fidelity is at the layer the inner loop does not exercise. Booting Talos nodes to iterate on a Go handler pays production's bootstrap cost on every recreate |
| Docker Desktop or Orbstack Kubernetes | bundled with the host tool | shares the host image store, so no load step at all | upstream, vendor-managed | The best image path of the field and no cluster lifecycle to control: it is one cluster, tied to a specific desktop product, which [ADR-0205](0205-environment-parity.md)'s per-engineer recreate-freely lifecycle needs |

**The distance from production is deliberate and bounded.** Parity is at the artifact layer, so what must match is the charts, the images, and the API — not the distribution ([ADR-0205](0205-environment-parity.md)). k3s and upstream Kubernetes are both conformant, so a chart that applies to one applies to the other, and the Talos-specific decisions in [ADR-0200](0200-cluster-topology.md) — machine config, system extensions, KubeSpan — have no laptop counterpart to diverge from.

The failure this could hide is a Kubernetes-version or CNI-behaviour difference, which is what the CI e2e tier exists to catch on the real thing.

**The ~20-service ceiling is the trigger to revisit.** "Run it all locally" is the consensus-correct choice below it and the consensus-wrong one above it. A project that grows past it moves to the third approach. The cheap thing to protect meanwhile is **trace-context propagation**, because request-level isolation (Lyft's staging overrides, Uber's SLATE, Signadot's sandboxes) is built on it. OTel is already wired; keeping propagation honest keeps that door open.

### Orchestrator

Live file sync is the headline feature of Skaffold, Tilt, DevSpace, and Okteto, and it is irrelevant here: **the inner loop runs the service natively on the host, so there is no container to sync into.** Four of the five orchestrators optimise for a problem this design does not have.

| Tool | Resolves per-service deps on a subset | Parallel config system | Verdict |
| --- | --- | --- | --- |
| Garden | **the only one that does** | `garden.yml` per action | Right convention, wrong dependency — see below |
| Skaffold | partial, module granularity | `skaffold.yaml` | Re-adopting it reverses [ADR-0101](0101-monorepo.md) for no capability gained |
| Tilt | **explicitly not** — documented behaviour | Starlark | The healthiest project in the set, and its subset mechanism ignores dependencies, which is the exact failure being fixed |
| DevSpace | coarse — cross-repo, always first | `devspace.yaml` | Wrong granularity |
| Okteto | no | `okteto.yml` | Remote-namespace oriented |
| **mise + bash + Helm + Argo** | no — **the gap** | none | **Chosen**, with the gap closed below *(reasoned)* |

**Garden's convention is adopted; Garden is not.** Two independent reasons:

- *Stewardship.* Garden's open-source repository is under corporate ownership and its commit and release cadence trails Tilt's and Skaffold's by an order of magnitude. Driver 6 makes active maintenance an entry gate rather than a scoring column, and it is not met.
- *Architecture.* Adopting Garden means adopting its whole config system: its own build, test, and deploy verbs and its own environment model. That does not sit beside Helm, Argo CD, and mise; it duplicates them, colliding with [ADR-0101](0101-monorepo.md) and [ADR-0201](0201-gitops.md). Two orchestrators would resolve a graph over six services.

A like-for-like sizing put Garden at roughly 210 fewer lines (~400 against ~610), consisting mostly of readiness-gated ordering and image build/import.

It does **not** win on graph resolution, the property that attracted us: mise already dedupes and parallelises. The three gnarliest pieces — docker-bridge discovery in the edge glue, SOPS secret materialisation, and the Argo pause/resume — have no Garden model and survive in bash either way.

### Local-process seam

| Tool | Cluster-side component | Modifies the workload | Gives the process pod env and files | Runs a non-containerised process |
| --- | --- | --- | --- | --- |
| mirrord | temporary agent pod | no | **yes** | yes |
| Telepresence | Traffic Manager + sidecar | yes | yes | yes, needs local root |
| Gefyra | none | no | no | **no** — Docker only, which is the entire inner loop |
| **Edge glue + port-forward** | none | no | no — a hand-copied `.env` | yes |

**mirrord is the option that closes the remaining weak cell** — giving a natively-run process the pod's environment and files — and its no-cluster-component design matches the constraints. It is compared here rather than adopted because the local-process seam works without it.

### API mock

| Tool | 3.1 support | Source of truth | Runtime added | Verdict |
| --- | --- | --- | --- | --- |
| **Prism (Stoplight)** | full | the spec — serves committed `examples`, generates from schemas otherwise | a container; no `mise` tool, no `package.json` | **Chosen** — request validation is free feedback: unknown route 404, bad body 422 *(documented)* |
| MSW | n/a | typed handlers | in-process | An in-process double, not a running API. Nothing outside the Next server can reach it. It owns the test layer, which is the different-concern justification driver 5 requires |
| Microcks | full | the spec, GitOps-native | **MongoDB** | The best GitOps fit and the only one that also does contract testing, but a datastore joining the floor for a development mock fails the budget rule |
| `muonsoft/openapi-mock` | **3.0 core (kin-openapi)** | the spec | none — a Go binary | Best language fit. Rejected as primary on a correctness risk: the specs are 3.1. The documented fallback if the vendored container proves unacceptable, gated on demonstrating 3.1 support |
| WireMock / Imposter | good | own DSL | **JVM** | Excluded by [ADR-0100](0100-language-and-runtime.md) |
| Mockoon | import-only | **its own JSON file** — the import is a fork | GUI | The spec stops being the source of truth on day two |
| Hoverfly | n/a | recorded traffic | none — Go | Needs real traffic to capture first. A complement to a real environment, not a substitute |
| Fake services from `ogen` | full | the spec | none | Highest fidelity including state, at a cost that is **per service** — across a fleet, a parallel implementation of the platform |
| No mock | n/a | n/a | none | The honest baseline. Does not survive a growing fleet, and does not help an engineer changing no service |

### Identity source

| Option | Why |
| --- | --- |
| **The real Kratos** | **Chosen.** Cheap, and its behaviour *is* the contract *(reasoned)* |
| A fake IdP — `mock-oauth2-server`, Dex, seeded Keycloak | The standard answer elsewhere, because OIDC is a **protocol boundary** where any conforming provider is interchangeable. Kratos is not that boundary: the frontend couples to Kratos's own surface — browser self-service flows, flow ids, `ui.nodes`, CSRF in flow state, opaque session cookies, `/sessions/whoami`. A convincing fake reimplements Kratos's flow state machine, which is a second implementation. This is a property of Kratos, not a ban: **Hydra** *is* a protocol boundary, so a fake OAuth2 provider is legitimate for third-party client work |
| An in-application bypass | Rejected on driver 2, and it saves nothing: a bypass that functions must return a fully-populated `Session` — identity id, email, roles, AAL — for the gates downstream of `whoami()`, so a hand-maintained fake identity survives anyway, without the login, expiry, or cookie behaviour that made keeping Kratos real worthwhile |

## Decision

### Two tiers

| Tier | Command | Parity | What runs | For |
| --- | --- | --- | --- | --- |
| **Inner loop** | `cluster:base` plus a service's own tasks | **interface** | the local floor; the service under change runs natively on the host | day-to-day coding, UI work |
| **Full platform** | `cluster:full` | **implementation** | the real platform charts at `instances=1` — CNPG, the Temporal chart, OpenFGA, SeaweedFS, observability, edge and auth | e2e ([ADR-0601](0601-testing-strategy.md)), pre-merge validation, CI, label-gated PR previews |

The inner loop runs the service natively against lightweight stand-ins — a plain Postgres, `temporal server start-dev`, in-memory OpenFGA — reached through `dev:forward`. The stand-ins honour the same wire contract, so a bug reproduced against them reproduces in production. There is no image build, redeploy, or file watch on the hot path.

The full platform runs the same charts production runs, scaled to one replica through the `local` values overlay. It catches operator behaviour, sync ordering, and chart wiring, and it is the exact configuration CI and PR previews use.

### Native and in-cluster are per service

| Mode | What runs | Cost | Limits |
| --- | --- | --- | --- |
| **Native** (`service:dev` + `mise run server`) | the service on the host, behind the real edge | lightest | no pod env or files; no NetworkPolicy enforcement |
| **In-cluster** (`cluster:add`) | the service from the working tree, in the cluster | image build per change | no debugger attach |

**Any number of services can be native at once**, each bound to its own registered port. Debugging a flow spanning three services means running all three natively with breakpoints in all three, while everything they call stays in the cluster.

That requires a **committed port registry** (`scripts/lib/ports.sh`), not a convention. Every server binds `:8080` in-cluster, which collides the moment a second runs on the host.

Ad-hoc ports would live in each engineer's gitignored `.env`, where they cannot be shared or defaulted. One stable port per service on every machine is what lets a caller's `CATALOG_URL` ship a working default. `lint:ports` enforces uniqueness and that each service binds what the registry assigns. `:8080` stays unassigned because the host maps it to the edge.

### Composition: a floor, plus per-service declarations

There are no named profiles. **A floor**: `cluster:base` brings up Traefik, cert-manager, Postgres, Kratos, and Oathkeeper, plus the host edge glue and the seeded test identities. It is unconditional.

The identity stack is in the floor deliberately. Kratos and Oathkeeper are cheap, and their behaviour — cookies, CSRF, session expiry, AAL, `401`/`403` — **is** the contract every service consumes.

A service reached through this edge gets real Oathkeeper identity headers; one curled directly on `:8080` is handed forged ones. A backend engineer who wants only Postgres still pays for the edge; that trade is accepted here rather than discovered later.

**Everything else is opt-in, declared by the service that needs it**, in its own `.mise.toml`:

| Family | Declares | Satisfied by | Guard |
| --- | --- | --- | --- |
| `dep:*` | infrastructure the service reads — Postgres, Temporal, OpenFGA | a label slice of `infra/local/deps.yaml` | Ready or not |
| `svc:*` | other services it calls over HTTP | the callee deployed and forwarded to its registered port | **answered or not** |

What is up is **base ∪ the declared dependencies of whatever is running**. There is no table to keep honest, because a service knows its own dependencies and a named profile never can.

`svc:*` exists because a microservices repo cannot omit the second family. The orders checkout saga calls catalog and payment ([ADR-0302](0302-temporal.md)), so a worker without them starts cleanly and dies on the first checkout — a failure later than a missing database and therefore harder to attribute.

**Deploying the callee is the default; running it natively is the override.** The usual case is working on the caller, where callees should be opaque. `svc:catalog` guards on *the port being answered*, not on a deployment existing, so starting catalog natively leaves the graph satisfied. The cross-service debugging case is the single-service case with more callees run by hand.

Dependency components are deliberately **not** addressable through `cluster:add`. `postgres` is both a platform chart and a `deps.yaml` slice, so making components user-facing nouns would be ambiguous. They stay graph-internal, which keeps `cluster:add` resolving over two disjoint namespaces — services and platform charts — and a name in both is a repo bug, not a user error.

### mise supplies the graph

A service's nested `.mise.toml` can depend on root-config tasks, diamond dependencies dedupe so a shared `cluster:base` runs once, and independent dependencies run in parallel. That is Garden's graph-execution semantics in a tool already pinned.

```toml
# root .mise.toml
[tasks."dep:temporal"]
depends = ["cluster:base"]
run = "bash scripts/dep-apply.sh temporal"

# services/orders/.mise.toml
[tasks.worker]
depends = ["env", "dep:postgres", "dep:temporal", "svc:catalog", "svc:payment"]
run = "go run ./cmd/worker"
```

| Layer | Owns |
| --- | --- |
| mise | the dependency graph and task vocabulary. Declarations only — no logic in `.mise.toml` |
| bash | one small idempotent installer per component (`dep-apply.sh`) or sibling (`svc-apply.sh`) |
| Helm | the component units — the same charts every tier |
| Argo CD | `cluster:full` only |

**The one property not free:** Garden does status checks and skips work already done. mise does not — its `sources`/`outputs` staleness is keyed on files, and "is Temporal already Ready?" is not a file. Without a guard, every `mise run server` re-pays a full apply and rollout wait.

The guard is therefore **centralised** in `dep-apply.sh` and `svc-apply.sh` rather than hand-rolled per component: one shared readiness check, about twenty lines, inherited by everything. A component that forgot its own guard would cost the inner loop its speed silently, which is why the guard does not live in the components.

### The service contract

Every service participates identically. Checked by `lint:service-contract`.

| A service must | Because |
| --- | --- |
| Live at `services/<name>/`, name matching everywhere | `cluster:add` resolves names over two disjoint namespaces |
| Expose `server`, `test`, `lint`, `build`, plus `worker`, `migrate`, `generate` when it has them | CI and `cluster:add` invoke them by name ([ADR-0101](0101-monorepo.md)) |
| Register a local port in `scripts/lib/ports.sh` and set the same `PORT` in its `.mise.toml` | So it runs natively alongside others and callers' `<SVC>_URL` have working defaults |
| Declare `dep:*` for the infrastructure it reads | The graph brings up exactly that, and the deploy path reads the same list |
| Declare `svc:*` for every service it calls over HTTP | Otherwise it starts cleanly and fails on the first cross-service call |
| Ship `.env.example` covering every variable it reads | `mise run server` seeds `.env` from it; an unlisted variable is a silent default |
| Ship a values file per environment under `infra/gitops/services/<env>/values/`, or declare `# platform/not-deployed: <env>` | The ApplicationSet generates one Argo Application per values file. No file means absent from that environment, and nothing reports it |
| Ship a `Dockerfile` and a `README.md` | The image is built by the same path in CI and `cluster:add` |
| Bind `httpmw.ListenAddr()` | `:8080` in-cluster — the chart's containerPort, the IngressRoutes, and the NetworkPolicies all assume it |

**The values-file rule needs an explicit opt-out because absence is not an error state in a git-directory generator.** A missing file produces no Application and no diagnostic. The failure it guards is silent and cross-service: `infra/auth/oathkeeper/values.yaml` points the `remote_json` authorizer at `authz-server` by service DNS in every environment, so an `authz` with no values file for an environment leaves every gated dashboard request there resolving to a Service that does not exist.

The deployment surface of this platform is the set of values files, and it is stated rather than inferred.

**The check cannot verify the declarations are true.** That a service listing `dep:temporal` uses Temporal at all, or that a new HTTP call gained its `svc:*` edge, is a reviewer's job. Adding a cross-service call without its edge is the most likely way to regress the local loop.

### GitOps locally

Argo CD reconciles committed git state, so it is **not** the inner loop's engine.

The **full tier does run Argo CD**: a local bootstrap (`infra/gitops/local-bootstrap/`) applies the same app-of-apps production uses, syncing committed `master`, so sync ordering, app discovery, and secret materialisation are exercised as in production. Only the two components Argo cannot self-create — the CNI and Argo CD itself — are installed imperatively before the root app.

Three escape hatches cover uncommitted infra:

| Change | Path |
| --- | --- |
| Chart or values | `platform:deploy -- <chart>` pauses Argo auto-sync on that one app and `helm upgrade`s from the working tree |
| GitOps wiring — sync-waves, ApplicationSets, App defs | push a branch and point the local root app's `targetRevision` at it, exercising the real delivery path |
| CNI or CRD | `cluster:delete` plus a fresh `cluster:full`. Hot-swapping a CNI on a live cluster blips networking — inherent to the component, not a tooling gap |

`cluster:remove` reverses `cluster:add` **and restores Argo auto-sync**, which the add path paused.

### The mock serves data; it never serves identity

**The mock has no authentication or authorization responsibility.** It issues no `401`, is unaware of sessions, and ignores every credential it receives.

There is nothing there to mock. Authentication is enforced outside OpenAPI — Oathkeeper validates at the edge and injects identity headers — so no service spec declares a `securityScheme`. A contract mock derives behaviour entirely from the spec, and the specs say every operation is open. Teaching the mock to emit `401`s means hand-authoring auth behaviour in a second place.

The same reasoning disposes of authorization. `GET /orders` returns the caller's orders, filtered by the identity header in the real service. A mock returning three arbitrary orders is **exactly as useful** for building the orders table — its columns, empty state, pagination, loading skeleton, error state. Ownership correctness is a property of the real service's query.

### The mock's input is the committed projection

The mock is served `internal.json` and nothing else: the frontend's edge surface by construction, merged, `servers: /api`, drift-checked.

**A glob over `services/*/openapi.yaml` is not an acceptable substitute.** It captures `services/authz/`, an all-`cluster` service whose `servers` is `/`, and `services/_template/`, which is scaffolding. Aggregating those and stamping `servers: /api` publishes `/api/identities` and `/api/items` — routes that do not exist at the real edge and must not, inverting the invariant `lint:api-audience` enforces. Using the projection also removes the merge step, its tooling, and its collision handling.

### Fidelity is bought in the contract, not the tool

The mock runs **examples-first**: committed `example` and `examples` values from the spec, falling back to schema generation where none exist. Schema-generated responses are a starting point — an array schema with no `minItems` legitimately generates an empty list, and an open object schema generates fields the real API never returns.

The fix belongs in the contract. Examples in `services/<service>/openapi.yaml` make mock responses deterministic and code-reviewed, and the same examples render in the Scalar developer portal — one investment, two consumers.

### UI work is the floor plus the mock

| Component | State on `cluster:base` | Why |
| --- | --- | --- |
| Traefik | real | routes `/api` to the mock and `/` to the host `next dev`; owns the same-origin contract |
| Kratos | real | login, logout, CSRF, cookie attributes, 7-day expiry, AAL |
| Oathkeeper | real | the `401`/`403` behaviour the mock cannot invent |
| cert-manager | real | mints the wildcard TLS the edge serves |
| Postgres (ephemeral) | real | Kratos's store |
| **The mock** | added on top | replaces every application service on `/api` |
| Temporal, OpenFGA, CNPG, observability, Go services | absent unless declared | none renders a page |

The application's authentication path is **byte-identical to production**. There is no bypass, no development provider, no `NODE_ENV` branch, because nothing needs one. For landing pages and logged-out surfaces the mock runs standalone with the auth stack down — less of the same tier, not a separate one.

### Identity comes from a seeded real login

`cluster:base` seeds the committed deterministic test identities [ADR-0601](0601-testing-strategy.md) defines, rather than inventing a parallel development-only identity, and `scripts/auth-token.sh` logs in by driving the real native flow. Kratos sessions last **7 days**, so a human logs in about once a week; the bootstrap exists so a fresh cluster is usable immediately and CI never types a password.

### The mock is local tooling only

It joins no tier in [`docs/operational-surface.md`](../operational-surface.md) — that inventory governs software the platform *operates*, and this runs only on an engineer's machine, the same status k6 holds. The omission is a decision.

It is forbidden anywhere correctness is asserted ([ADR-0601](0601-testing-strategy.md), driver 4): never in `mise run test`, `ci:affected`, the e2e or visual suites, or any deployed environment. Its only consumer is a human looking at a browser.

The mock's validating-proxy mode is not used either, because the platform answers that question twice already: the server is generated from the spec by ogen, and every generated Go client validates responses against it, so a spec-violating response fails the integration tests that drive it.

What escapes both is the edge's own error responses — `401`, `403`, `429` from Traefik and Oathkeeper — which reach no generated client. That fixed, small set is asserted against the Problem envelope in e2e.

### Local domain and local-only manifests

`*.localtest.me` is real public DNS resolving to `127.0.0.1` with zero host configuration. `.local` is reserved for mDNS ([RFC 6762](https://www.rfc-editor.org/rfc/rfc6762)) and collides with Avahi and Bonjour; `*.localhost` and `*.test` signal "local" more loudly but need a modern resolver or a hosts entry.

A short enumerated set of manifests has no production analogue:

| File | Why it is local-only |
| --- | --- |
| `infra/local/deps.yaml` | the inner-loop dependency stand-ins. The full tier does not use it |
| `infra/local/traefik-config.yaml` | tunes the bundled Traefik for cross-namespace refs. Production runs its own |
| `infra/local/edge-auth.yaml` | routes `/auth` and landing to a host-run `next dev` |
| `infra/local/mock.yaml` | the mock Deployment, Service, and `/api` IngressRoute, carrying the real edge middleware chain. The spec ConfigMap is stamped from the committed projection on every run |
| `infra/local/coredns-rewrite.yaml` | resolves the env host to Traefik from inside the cluster, since `127.0.0.1` in a pod is the pod's own loopback |
| A local CA issuing the wildcard | the same cert-manager mechanism with a local ClusterIssuer. It is a **CA**, not a self-signed leaf: a leaf cannot be a trust anchor, which would force `NODE_TLS_REJECT_UNAUTHORIZED=0` and block the frontend from running as its production image locally |

## Consequences

### Positive

- The inner loop stays fast: above the floor, a service pays only for what it declares.
- The full tier validates the same software production runs — operators, sync ordering, chart wiring — before a change reaches a deployed environment.
- The frontend has a tier costing a fraction of `cluster:full` with the authentication path untouched.
- No development-only code ships in `apps/frontend/`. The class of bug where a screen works locally and `403`s in staging cannot occur, because the local gates are the real gates.
- The mock cannot drift from the contract: its only input is a drift-checked artifact.
- Examples serve the developer portal and the mock simultaneously; the test-identity bootstrap has two consumers, so it is exercised daily rather than nightly.
- No tool to learn. A contributor who knows bash, Helm, and mise can read the whole dev loop.

### Negative / Risks

- **We own the dependency resolution Garden gives for free.** A small graph and a fair trade, which must not grow into a general-purpose engine by accident. The discipline: `.mise.toml` files stay declarations, and each bash script installs exactly one component. Shell logic accumulating in task definitions means rebuilding Garden badly.
- **`svc:*` owns the one long-lived process the dev loop runs.** A mise task must return for the graph to continue, but the callee's port-forward must outlive it, so `svc-apply.sh` detaches one and tracks it by pidfile, reaped by `cluster:remove`. This is the least elegant part of the design and the price of not adopting a process manager; it is confined to one script. If it becomes a source of bugs, mirrord subsumes it.
- **A service's callee list is load-bearing**, and nothing detects an omitted `svc:*` edge — the call fails at runtime.
- **A third vendored Node tool.** Prism's image is third-party Node software the repo runs but does not author, alongside the Playwright runner and the Lowdefy console. Scope is hard: a local-only container, no Node in any `.mise.toml`, no `package.json`, no lockfile, never in an image built from our source.
- **The mock is stateless.** A create followed by a read does not reflect the write, and a `WorkflowHandle`'s `result_url` polls nothing. This is the tier's honest boundary.
- **Examples are a maintenance surface.** `response-example-required` enforces that a `2xx` response has an example; nothing enforces the example is still true after a schema change. Mitigated by keeping examples minimal and reviewing them as part of the contract diff — a wrong example is a wrong contract, visible in the same PR.
- **The floor is not free.** Traefik, cert-manager, Kratos, Oathkeeper, and Postgres must be up to run any service at all. That cost buys the absence of every development-only auth branch.
- **The full tier costs laptop RAM.** Per-service declarations exist so day-to-day work runs only its slice.
- **Two mocking mechanisms exist** — MSW at the test layer, Prism at the dev-loop layer. Bounded by an explicit layer boundary and by neither being permitted in e2e.
- **Re-evaluate at the ~20-service ceiling**, or if the graph outgrows a declaration plus one shared installer. Both are visible, not gradual.

## Rules

- Local development runs two tiers: the `cluster:base` inner loop and `cluster:full`. There are no named profiles.
- What is up locally is the floor plus the declared dependencies of what is running. A service declares `dep:*` for infrastructure and `svc:*` for every service it calls over HTTP. `(CI: lint:service-contract)`
- `.mise.toml` files carry declarations only. Component logic lives in one idempotent installer script per component, each fast-exiting when already satisfied.
- Every service registers a local port in `scripts/lib/ports.sh` and binds `httpmw.ListenAddr()`; `:8080` stays unassigned. `(CI: lint:ports)`
- Every service ships a values file per environment or declares `# platform/not-deployed: <env>`. Absence is never inferred. `(CI: lint:service-contract)`
- Argo CD is the engine for `cluster:full` only. Uncommitted infra iterates through `platform:deploy` or a branch `targetRevision`, never by editing cluster state directly.
- API mocking exists for the UI development loop only. The mock appears in no deployed environment, no chart, and no image built from our own source.
- The mock's only input is the committed `internal.json` projection. Globbing `services/*/openapi.yaml`, hand-written route files, and standalone fixture bodies are not used.
- The mock serves no authentication or authorization behaviour: no `401`, no session awareness, no identity headers.
- The application contains no development-only authentication code. A session bypass, a fake session object, or an environment-conditional branch in the session path is a review-blocker. An environment-conditional branch that grants nothing is not. `(CI: lint:auth-inline)`
- Authenticated local development uses the real Kratos with the committed test identity. Fake identity providers are not used for Kratos; they remain permitted for Hydra, which is a protocol boundary.
- The mock runs examples-first. Schema generation is a flag-gated fallback for an endpoint without one.
- The mock is forbidden in `mise run test`, `ci:affected`, and the e2e and visual suites.
- The mock ships as a vendored container, adding no `package.json`, lockfile, or `node_modules`. `(CI: lint:node-scope)`
- The mock is local tooling and joins no tier in [`docs/operational-surface.md`](../operational-surface.md).
- Statelessness is the tier's boundary: persistence, workflow progression, and authorization decisions are exercised against real services, never asserted against the mock.
