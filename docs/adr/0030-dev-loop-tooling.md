# ADR-0030: Dev-Loop Tooling

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Platform team
- **Related:** [ADR-0002](0002-monorepo.md), [ADR-0003](0003-cluster-topology.md), [ADR-0004](0004-gitops.md), [ADR-0016](0016-environment-parity.md), [ADR-0029](0029-api-mocking-and-ui-dev-loop.md)

## Context

[ADR-0016](0016-environment-parity.md) settles *what* the local model is: a `cluster:base` floor, plus dependency
components each service declares for itself, plus a service running either natively or in-cluster. This ADR settles
*what builds it*, because the obvious answer — adopt one of the mature Kubernetes dev-loop tools — deserved a real
evaluation rather than a reflex.

The specific capability we needed and did not have was **dependency resolution**: "bring up `orders`" should also
bring up Temporal and OpenFGA, in order, without the engineer knowing that list. Deploying onto a bare floor without
it silently CrashLoops.

An earlier revision of this repo used Skaffold and removed it ([ADR-0002](0002-monorepo.md)), so re-adopting an
orchestrator would reverse a decision already made. That raised the bar but did not settle the question.

## Decision drivers

1. **The gap is dependency resolution, nothing else.** Anything a tool offers beyond that is weight, not value.
2. **Coexistence with Helm and ArgoCD.** [ADR-0004](0004-gitops.md) makes Argo the delivery engine. A second
   orchestrator that also wants to own the same resources is a conflict, not an addition.
3. **This is a template.** Every project derived from it inherits the toolchain, and every contributor to those
   projects must learn it. A dependency here is heavier than in a normal repo.
4. **Active maintenance is an entry gate**, not a scoring column — an unmaintained project is not a candidate.
   License is likewise a gate (must be OSS). Popularity is not a fitness measure and is excluded.

## Decisions

### The evaluation

Two categories, not one. They are not alternatives to each other, and this design needs an answer in both.

**A note that removes most columns:** live sync / file sync is the headline feature of Skaffold, Tilt, DevSpace and
Okteto — and it is irrelevant here, because our inner loop runs the service **natively on the host**. There is no
container to sync into. Four of the five orchestrators are optimised for a problem this design does not have.

#### Orchestrators

| Tool | Resolves per-service deps on a subset | Coexists with Helm + ArgoCD | Requires a parallel config system | Solves a gap we have |
|---|---|---|---|---|
| **Garden** | ✅ the only one | Wraps them as providers | ✅ `garden.yml` per action | Yes — dependency resolution |
| **Skaffold** | Partial (module granularity) | Deploys via them | ✅ `skaffold.yaml` | No — already removed once (ADR-0002) |
| **Tilt** | ❌ explicitly not | Deploys via them | ✅ Starlark | No |
| **DevSpace** | Coarse (cross-repo, always-first) | Deploys via them | ✅ `devspace.yaml` | No |
| **Okteto** | ❌ | Via deploy commands | ✅ `okteto.yml` | No — remote-namespace oriented |
| **Status quo** (mise + bash + Helm + Argo) | ❌ *(the gap)* | It *is* them | ❌ | — |

#### Local-process seam

| Tool | Cluster-side component | Modifies the workload | Gives the process pod env + files | Runs a non-containerized process |
|---|---|---|---|---|
| **mirrord** | Temporary agent pod | ❌ | ✅ | ✅ |
| **Telepresence** | Traffic Manager + sidecar | ✅ | ✅ | ✅ (needs root locally) |
| **Gefyra** | None | ❌ | ❌ | ❌ Docker only |
| **Status quo** (edge glue + port-forward) | None | ❌ | ❌ *(manual `.env`)* | ✅ |

### We take Garden's convention, not Garden

Garden is the only tool that solves our actual gap, and its convention is the right one: **dependencies declared with
the service, resolved by one shared engine.** We adopt that shape and decline the tool, for two independent reasons.

**Stewardship.** Incredibuild acquired Garden in November 2024 (~$65M, with an 11% staff cut). As of August 2026 the
OSS repository shows 4 commits in 90 days and no release since February 2026 — an order of magnitude below Tilt or
Skaffold. That fails the maintenance gate for a template other projects are built on.

**Architecture.** Adopting Garden means adopting its whole config system — a `garden.yml` per action, its own
build/test/deploy verbs, its own environment model. That does not sit *beside* Helm, ArgoCD and mise; it substantially
duplicates them, colliding with [ADR-0002](0002-monorepo.md) (bash as the glue) and [ADR-0004](0004-gitops.md) (Argo
as the delivery engine). We would be running two orchestrators to resolve a graph over six services.

A like-for-like sizing of the dev-loop surface put Garden at roughly 210 fewer lines (~400 vs ~610) — real, but
consisting mostly of readiness-gated ordering and image build/import. It does **not** win on graph resolution, the
thing that attracted us: mise already dedupes and parallelises. And the three gnarliest pieces — the edge glue's
docker-bridge discovery, the SOPS secret materialisation, and the ArgoCD pause/resume — have no Garden model and
survive in bash either way.

### mise supplies the graph

Verified against a scratch project before committing to it: a service's nested `.mise.toml` **can** depend on
root-config tasks, diamond dependencies **dedupe** (a shared `cluster:base` runs exactly once), and independent
dependencies **run in parallel**. That is Garden's graph execution semantics in a tool already pinned.

```toml
# root .mise.toml
[tasks."dep:temporal"]
depends = ["cluster:base"]
run = "bash scripts/dep-apply.sh temporal"

[tasks."svc:catalog"]
depends = ["cluster:base"]
run = "bash scripts/svc-apply.sh catalog"

# services/orders/.mise.toml
[tasks.worker]
depends = ["env", "dep:postgres", "dep:temporal", "svc:catalog", "svc:payment"]
run = "go run ./cmd/worker"
```

The two families are the same idea applied to the two kinds of thing a service needs: `dep:*` for infrastructure,
`svc:*` for the other services it calls. Both are declared with the service, both carry the guard, and both are opaque
to the caller. Keeping them as separate namespaces rather than one is what lets the guards differ — a `dep:*` is Ready
or not, while a `svc:*` is *answered or not*, which is what lets a natively-run callee satisfy the same edge.

| Layer | Owns |
|---|---|
| **mise** | The dependency graph and task vocabulary. Declarations only — no logic in `.mise.toml` |
| **bash** | One small idempotent installer per component (`dep-apply.sh`) or sibling (`svc-apply.sh`), each fast-exiting when already satisfied |
| **Helm** | The component units — unchanged, same charts every tier |
| **ArgoCD** | `cluster:full` only — unchanged |

### The one property we do not get for free

Garden does **status checks**: it skips work already done. mise does not — its `sources`/`outputs` staleness skipping
is keyed on *files*, and "is Temporal already Ready?" is not a file. Without a guard, every `mise run server` re-pays a
full apply and rollout wait for every dependency, and this model ends up slower than the one it replaced.

So the guard is centralised — in `scripts/dep-apply.sh` for components, `scripts/svc-apply.sh` for siblings — rather
than hand-rolled per item. One shared readiness
check, inherited by every component, recovers most of the safety property for about twenty lines. **A single component
forgetting its guard would silently cost the inner loop its speed**, which is exactly why it does not live in the
components.

### Rejected alternatives

- **Tilt** — the strongest rejection, and the healthiest project in the set. Starlark could carry the graph. But it
  duplicates Helm and Argo, and its subset mechanism explicitly does *not* consider dependencies (documented behaviour,
  not an oversight) — which is the precise failure mode we set out to fix.
- **Skaffold** — active despite a widely-repeated "maintenance mode" claim that no longer appears in its README, but
  re-adopting it reverses [ADR-0002](0002-monorepo.md) for no capability we lack.
- **mirrord** — genuinely attractive and worth revisiting. Its no-cluster-component design matches our instincts, and
  it would close the one cell where our seam is weakest: giving a natively-run process the pod's environment and
  files, which today is a hand-copied `.env`. Nothing is blocked on it, so it stays a candidate rather than a
  dependency.
- **Telepresence / Gefyra** — Telepresence needs local root and a sidecar per pod; Gefyra cannot run a
  non-containerized process, which is our entire inner loop.

## Consequences

- **We own the dependency resolution Garden gives for free.** At six services that is a small graph and a fair trade.
  It must not be allowed to grow into a general-purpose engine by accident.
- **The discipline that keeps this from rotting:** `.mise.toml` files stay declarations, and each bash script installs
  exactly one component. The moment shell logic accumulates in task definitions, we are rebuilding Garden badly.
- **No tool to learn.** A contributor who knows bash, Helm and mise can read the whole dev loop.
- **Re-evaluate if the service count approaches ADR-0016's ~20-service ceiling**, or if the graph outgrows what a
  declaration plus one shared installer can express. Both are visible, not gradual.
- **mirrord remains the named next step** if the manual-`.env` friction becomes the dominant complaint.

## References

- [Signadot — Local Development on Kubernetes: The Complete Guide (2026)](https://www.signadot.com/guide-to-local-development-kubernetes/)
- [Kubernetes Blog — Comparing Telepresence, Gefyra, and mirrord](https://kubernetes.io/blog/2023/09/12/local-k8s-development-tools/)
- [Tilt Blog — Running just a few of your many services should be easy](https://blog.tilt.dev/2022/03/03/resource-catalog.html)
- [Garden Docs — Graph Execution](https://docs.garden.io/contributing-to-garden/graph-execution)
- [Incredibuild acquires Garden](https://www.incredibuild.com/news/incredibuild-acquires-garden)
- [Lyft Engineering — Extending our Envoy mesh with staging overrides](https://eng.lyft.com/scaling-productivity-on-microservices-at-lyft-part-3-extending-our-envoy-mesh-with-staging-fdaafafca82f)
- [Uber Blog — Simplifying Developer Testing Through SLATE](https://www.uber.com/us/en/blog/simplifying-developer-testing-through-slate/)
