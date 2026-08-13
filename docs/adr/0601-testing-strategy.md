# ADR-0601: Testing Strategy

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0100](0100-language-and-runtime.md), [ADR-0101](0101-monorepo.md), [ADR-0204](0204-resource-management.md), [ADR-0205](0205-environment-parity.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0400](0400-frontend.md), [ADR-0500](0500-observability.md), [ADR-0600](0600-local-development-loop.md)
- **Decides:** Correctness is layered from unit tests up to e2e against the full local platform, with load testing beside those layers rather than gating them.

## Context

Other ADRs pin pieces of testing and none owns the whole — [ADR-0101](0101-monorepo.md) the task names and affected-detection, [ADR-0400](0400-frontend.md) frontend unit and component tests, [ADR-0205](0205-environment-parity.md) the tier e2e runs on, and [ADR-0304](0304-identity-and-authorization.md) its own authz conformance suite. This ADR owns the cross-cutting question: what is the acceptance test for "the platform works", where does it live, what drives it, and when does it run.

Testing answers two separable questions, and conflating them produces a suite that answers neither:

| Question | Verdict | Load |
| --- | --- | --- |
| **Is it correct?** | works / broken | about one user |
| **What does it cost, and where does it break?** | within budget / regressed / saturated | deliberately past the knee |

Three accepted decisions depend on the second answer:

- [ADR-0204](0204-resource-management.md) requires CPU and memory requests per container and makes HPA opt-in "on a documented sustained-load signal", both because nothing produces the number.
- [ADR-0500](0500-observability.md) and [ADR-0501](0501-operator-uis-and-dashboards.md) define the apparatus that observes saturation — the capacity row, the `ClusterCPURequestsCommitted` and `NodeMemoryPressure` alerts — and an alert nobody has seen fire is an alert nobody knows the shape of.
- [ADR-0000](0000-platform-foundations.md)'s claim that **fixed platform cost dominates variable application cost** is quantitative, and this is what quantifies it.

Measured on `cluster:full`, one node, during the e2e suite, and re-derivable from the `load-test` dashboard: **no business service appears among the top consumers of memory or CPU**, and the observability stack alone outweighs every service put together. At about one user the cost is entirely platform.

The absolute footprint grows with the fleet and with log volume; the shape does not, which is what the tiering below rests on.

## Decision drivers

1. **The rendered, authenticated UI is the ultimate guarantee.** A working dashboard behind a real session proves edge, auth, services, and data are wired. Browser acceptance is the gauge; cheaper checks exist to localise a failure faster.
2. **One tool, one way** ([ADR-0000](0000-platform-foundations.md), principle 5). One engine drives product journeys, operator dashboards, and visual regression.
3. **Cost is the bring-up, not the test.** Every full-platform suite shares one expensive prerequisite, so cadence amortises one bring-up across every suite that needs it.
4. **No mocks in acceptance.** E2e runs against real services.
5. **Per-service cost.** Automatic full-platform bring-up on every PR does not survive a growing fleet.
6. **No new always-on component and no new runtime** for load testing. A generator used minutes per week must not join the Core floor ([`docs/operational-surface.md`](../operational-surface.md)).
7. **Results land in the stack already operated.** A load tool with its own metrics store, dashboards, and query language is a second observability plane.

## Considered options

### Browser, e2e, and visual engine

| Option | Cross-origin | Visual regression | Runtime | Verdict |
| --- | --- | --- | --- | --- |
| **Playwright (TypeScript)** | native, out-of-process driver | built in ([`toHaveScreenshot`](https://playwright.dev/docs/test-snapshots)) | Node | **Chosen** — lowest-flake auto-waiting for heavy SPA dashboards, and it crosses the Kratos redirect and `*.ops.<host>` subdomains ([ADR-0306](0306-trust-tiers-and-urls.md)) natively *(documented)* |
| WebdriverIO | BiDi | bolt-on service | Node | Higher baseline flakiness and bolt-on visual land exactly where sensitivity is highest. Its decisive edge, native mobile, is unused. Buys back no runtime purity |
| Cypress | fights it — single browser context | good | Node | The auth-redirect and ops-subdomain topology is its weak spot |
| Puppeteer | CDP, and Chromium-first | **none** | Node | The automation library much of this field is built on, without a runner, assertions, retries, or a baseline flow. Playwright is that library plus the parts a suite needs |
| Rod / chromedp (pure Go) | workable | **none** — pixel diffing, baselines, and a review flow would be hand-rolled | Go | The only Node-free path, rejected on the two capabilities needed most. Raw-CDP auto-wait is weaker on heavy dashboards |
| Stagehand, Midscene, Shortest | inherited | inherited | Node | Built on Playwright or CDP, or LLM-in-the-loop nondeterministic and cloud-tilted. They wrap an engine that must still be chosen |

### Load generator

| Option | Runtime added | Scenario artefact | Metrics path | Verdict |
| --- | --- | --- | --- | --- |
| **k6 (Grafana)** | none — a single static Go binary with an embedded JS engine (Sobek) | committed JS files | built-in OTLP output | **Chosen** — thresholds set the exit code, so a run is a CI gate without a wrapper *(documented)* |
| Locust | Python: a third toolchain, a pip tree, an image | committed Python | its own store, needs an exporter sidecar | Fails drivers 6 and 7 |
| Gatling | JVM (Scala/Java DSL) | committed DSL | HTML report — a parallel offline results plane | Strongest reporting in the field, rejected on drivers 6 and 7 |
| JMeter | JVM | **GUI-authored XML blob** | its own | Reviews as a blob, against config-as-code ([ADR-0000](0000-platform-foundations.md), principle 1) |
| Vegeta | none — Go | URL list | its own | A constant-rate URL hitter. The interesting path is multi-step and stateful, which it models only by shelling around it |
| Write one in Go | none | Go | ours | Reimplements VU scheduling, ramping executors, percentile aggregation, and threshold evaluation to avoid one pinned binary |

### Where a service integration test gets its dependencies

| Option | Postgres, Temporal, OpenFGA come from | Fidelity to production | Cost per run | Verdict |
| --- | --- | --- | --- | --- |
| **`cluster:base` plus the service's declared components** | the same charts production runs ([ADR-0205](0205-environment-parity.md)) | **the operators, the CRDs, and the network policy** | one cluster, shared by every test in the run | **Chosen.** The dependencies are already declared per service for deployment, so the test environment is derived from the deploy manifest rather than described twice *(reasoned)* |
| testcontainers-go | a container per dependency, started by the test process | plain images: no CNPG, no operator behaviour, no NetworkPolicy | one container set per package, torn down after | The industry default for Go service tests, and it would require each service to declare its dependencies a second time in Go. It also cannot exercise the operator-managed behaviour — failover, pooler, seeded authz model — that this platform's data tier has |
| A shared long-lived test database | a persistent environment | high | none per run, and cross-test interference forever | State leaks between runs, and a failing test becomes a question about who else was running |
| Mocks at the repository boundary | nothing | none — the SQL is never executed | fastest | It tests the code against its own assumptions about Postgres, which is the layer these tests exist to check |

**testcontainers is the strongest rejected option**, and the reason is specific: this platform's dependencies are operator-managed, so a plain `postgres:17` container is not what production runs. Where the platform to be tested against is plain images, that verdict reverses.

### Contract testing

| Option | What it verifies | Where the truth lives | Verdict |
| --- | --- | --- | --- |
| **The generated client, compiled against the spec** | that consumer and provider agree, at build time | the OpenAPI spec ([ADR-0303](0303-api-contracts-and-lifecycle.md)) | **Chosen by inheritance.** Every consumer calls through generated code, and CI fails on stale generation, so a contract break is a compile error rather than a test failure *(reasoned)* |
| Pact, or another consumer-driven contract broker | that a consumer's expectations still hold | consumer-written pacts, plus a broker to store them | Consumer-driven contracts solve a problem this repo does not have: consumers that ship separately from providers. Here they ship in the same commit ([ADR-0103](0103-release-and-versioning.md)), and the broker is a component |
| Microcks as a contract test | that a running provider matches the spec | the spec | Already compared as a mock in [ADR-0600](0600-local-development-loop.md) and rejected there on its MongoDB dependency; the same cost applies here |
| schemathesis or another spec-fuzzer | that the provider handles inputs the spec permits | the spec | Genuinely additive rather than an alternative: it tests robustness where the generated client tests agreement |

### Runtime for the Playwright runner

Bun is the sole JS runtime ([ADR-0100](0100-language-and-runtime.md)) and **cannot reliably run a browser test runner**.

Playwright, like every Node browser runner, spawns the browser with extra file descriptors (fd 3/4) as a pipe transport and forks workers over Node IPC — the least-travelled corners of `child_process` that Bun has not matched. Browser launch hangs or segfaults, and the runner produces no output. This is a Node-ecosystem-wide gap, not a Playwright bug, and not a "temporary" to build on ([ADR-0000](0000-platform-foundations.md)).

Node is therefore sanctioned as a **test-only escape hatch**, scoped to the e2e and visual runner alone.

## Decision

### Correctness layers

| Layer | Tool | Environment | Role |
| --- | --- | --- | --- |
| Unit / component | `go test`, `bun test` | none / `happy-dom` | logic and component shape in isolation |
| Service integration | `go test` + generated SDK clients ([ADR-0303](0303-api-contracts-and-lifecycle.md)) | `cluster:base` plus the service's declared components | one service against real Postgres, Temporal, OpenFGA |
| Preflight readiness | Go / shell | `cluster:full` | failure **localiser** — pods ready, ports open, Postgres and Oathkeeper reachable |
| **Browser acceptance** | **Playwright** | `cluster:full` | **the gauge** — product journeys and operator dashboards behind a real AAL2 session |
| Visual regression | Playwright `toHaveScreenshot` | `cluster:full` / static render | component shape against committed baselines |

Preflight runs before the browser suite so a red e2e reads immediately as "infra down" rather than "app broken". It is triage, not a competing acceptance test. The browser test is the final word.

**These are layers, not a pyramid.** Counted by tests rather than by scope, the shape is closer to a honeycomb: a thin base, a thick middle, and a top that carries the verdict. Three properties put it there:

- **The mock-heavy integration tier a pyramid thickens has little left to catch.** Clients and validators are generated from the spec and drift-checked ([ADR-0303](0303-api-contracts-and-lifecycle.md)).
- **The failures that cost this platform are cross-service and auth-shaped** — a header injected at the edge, an AAL2 session, an OpenFGA tuple. None is observable below the browser layer.
- **The usual objection to a heavy top does not hold here.** `cluster:full` runs the same charts as production ([ADR-0205](0205-environment-parity.md)), so end-to-end is not running against a fiction.

### Load is a fourth concern, not a fifth layer

Load testing sits beside these layers, not on them. A red load run means "slower than the budget", not "broken", so **performance tests are not part of `mise run test`, `ci:affected`, or the e2e suites, and never implicitly gate a merge**.

### Layout

| Workspace | Contents |
| --- | --- |
| `test/e2e/platform/` | cross-service product journeys (register → catalog → order → pay) and operator-dashboard journeys behind an AAL2 session |
| `test/e2e/frontend/(landing\|panel\|devportal)/` | per-route-group frontend suites |
| `test/e2e/visual/` | component visual regression against committed baselines |
| `test/perf/lib/` | shared configuration, target resolution, thresholds, response checks |
| `test/perf/scenarios/` | one runnable k6 script per load shape |
| `test/perf/seed/` | bulk data provisioning, so read paths meet a realistic table |

All e2e and visual tests share one Playwright config in the repo-root `test/e2e/` workspace.

### Runtime containment

Node is pinned in `test/e2e/.mise.toml`, never the root toolchain, so it installs only for a developer who runs the suite. k6 is pinned in `test/perf/.mise.toml` as an island tool; root `perf*` tasks delegate with `mise run -C test/perf …`.

k6's scenario language is JavaScript on its own embedded engine. **This does not extend the Node escape hatch**: `test/perf/` carries no `package.json`, no lockfile, no `node_modules`, and no Node binary. `lint:node-scope` asserts npm exists only in `test/e2e/`.

### Load scenarios

Both committed scenarios drive **through the edge** — Traefik → Oathkeeper → service — not a port-forwarded pod, because the edge is part of the system under test and [ADR-0305](0305-edge-auth-and-traffic-policy.md) puts a forward-auth hop on every request.

| Scenario | Path | Ceiling it finds |
| --- | --- | --- |
| `browse` | `GET /api/products`, `GET /api/products/{id}` | edge and connection pool — cheap per request |
| `checkout` | `POST /api/orders`, then poll to terminal status | workflow throughput — edge → orders → Temporal saga → catalog + payment |

Four profiles are selected by `PROFILE`, as data rather than duplicated files: **`smoke`** (1 VU, seconds — proves the script and target are wired), **`load`** (the steady baseline regressions are measured against), **`stress`** (ramp past the knee until thresholds break), **`soak`** (sustained, for leak detection).

### Load results reach Prometheus through the collector

k6 runs with `--out opentelemetry`, exporting to the existing OTel collector. This holds [ADR-0500](0500-observability.md)'s invariant — the only path to Prometheus is an OTLP push — so a load run needs no remote-write receiver and no scrape config.

- Series are namespaced `k6_*` (`K6_OTEL_METRIC_PREFIX`) and carry `service_name="k6"`, so they sit next to `k8s_pod_cpu_usage` on one time axis without being mistaken for a service's own telemetry.
- k6's `url`, `name`, and `error` system tags are **not** exported. They are unbounded by construction — a URL label mints one series per product id — and are replaced by a hand-written `endpoint` route-template tag.
- `--no-usage-report` disables anonymous usage reporting, matching the phone-home posture applied to Temporal and the object store.

The **`load-test` dashboard** correlates k6 throughput, latency percentiles, and error rate against pod CPU, memory, and limit utilisation. It sits **outside the L1–L3 triage funnel** ([ADR-0501](0501-operator-uis-and-dashboards.md)): that funnel diagnoses an unplanned incident, whereas a load test is a planned experiment where the operator knows what they are looking at. It is reached from the `perf` task output, not from Overview.

### Where load comes from

The default runner is k6 on the developer or CI machine, hitting the edge. It adds nothing to the cluster, and the generator competes for host CPU with the node, so **local numbers are a regression signal and a saturation-shape signal, never an absolute capacity figure**.

**Scale swap:** `k6-operator`, generating load from inside the cluster across multiple pods. **Trigger:** k6 reports `dropped_iterations` > 0 at the target rate, or absolute capacity figures are needed. **Seam:** scenarios are unchanged; only the place the VUs run changes.

### Cadence

| Suite | Trigger | Contents |
| --- | --- | --- |
| Unit / component / integration | per-PR, affected-scoped ([ADR-0101](0101-monorepo.md)) | `go test`, `bun test`, deps-only integration |
| `e2e:smoke` | per-PR, **label-gated** | one product golden path plus a key dashboard render |
| `perf:smoke` | per-PR, label-gated, alongside `e2e:smoke` | the scenarios still run, ~30s |
| `e2e` | nightly (activity-gated) + pre-release ([ADR-0103](0103-release-and-versioning.md)) | every journey, every operator dashboard, all visual baselines |
| `perf` (`load`) | nightly (activity-gated) + pre-release | the tracked baseline number |
| `perf:stress` | on demand, before a capacity decision | find the knee; produce the [ADR-0204](0204-resource-management.md) sizing and HPA signal |

Affected-detection scopes the cheap per-PR layers. The full-platform suites are their own CI jobs and are not part of `ci:affected`, because every e2e crosses service boundaries by nature.

Load thresholds live in the scenario files and are **budgets, not SLOs** — deliberately looser than the [ADR-0500](0500-observability.md) service SLOs, because a load run pushes into territory where the SLO is expected to break. A threshold breach fails the run; a nightly breach is a regression to triage, not a rollback trigger.

### Test data

Kratos starts with an empty identity store and no seeded user, and mail goes to the non-production sink ([ADR-0307](0307-outbound-email.md)) rather than to a recipient. E2e ships a **committed deterministic test-identity bootstrap** — an AAL1 product user and an AAL2 operator — provisioned identically in CI and locally, the same throwaway-credential pattern SOPS uses for the local age key ([ADR-0205](0205-environment-parity.md)).

No test depends on hand-created state. The same bootstrap provisions the identity the `edge` development profile logs in as ([ADR-0600](0600-local-development-loop.md)), so it is exercised daily rather than only nightly.

Load runs generate real data: a checkout run creates real orders and Temporal executions in whatever environment it targets. `test/perf/` refuses to run against a target it was not explicitly pointed at, and seeded data carries a recognisable prefix so it can be identified and removed.

### Visual baselines

The CI gate is committed accepted-snapshot diffing against baselines in `test/e2e/visual/`; an intentional UI change updates the baseline in the same PR. **Figma stays the authoring source of truth** ([ADR-0400](0400-frontend.md)), reviewed by humans. Rendered-versus-Figma diffing is too brittle — font hinting, anti-aliasing — to gate on.

**Deferred:** component-isolation tooling (Storybook) and a hosted review UI (Argos). **Trigger:** built-in baseline diffing stops scaling. **Seam:** both consume the same committed baselines, so adoption is additive.

## Consequences

### Positive

- One acceptance gauge — a rendered, authenticated UI — and one tool driving it.
- Operator dashboards are first-class tests: a green run proves edge, auth, services, and data end to end.
- One bring-up amortised across every full-platform suite keeps the heavy tier affordable.
- [ADR-0204](0204-resource-management.md)'s requests, limits, and HPA triggers stop being judgement calls.
- The capacity alerts and the Overview capacity row become testable before an incident tests them.
- Load adds no always-on component, no new runtime, and no second metrics plane — one pinned binary and text files.

### Negative / Risks

- **Node returns as a sanctioned runtime.** Contained to the `test/e2e/` runner and CI, never a service, app or library code, or an image built from our own source. ([ADR-0100](0100-language-and-runtime.md) sanctions one other Node island for the same vendored-tool reason: the Lowdefy admin console, [ADR-0401](0401-internal-admin.md).)
- **Label-gated smoke means an unlabelled PR gets no full-platform signal until nightly**, so a cross-service break can sit in `master` for up to ~24h. Accepted as the cost of not paying a full bring-up per PR.
- **Operator-dashboard e2e lives only in the nightly suite**, so platform-contract regressions surface within 24h.
- **The gauge is also the bottleneck.** Putting the verdict at the slowest, most flake-prone layer means a broken top blocks the signal entirely. Preflight localises the cause; it does not make the layer faster or steadier.
- **Heavy SPA dashboards can be flaky.** Mitigated by Playwright auto-wait and the preflight gate.
- **Local load numbers are not capacity numbers.** Stated wherever results are reported, and resolved by the k6-operator swap when absolute figures are needed.
- **A single local node is not a production topology.** Saturation shapes transfer; absolute ceilings do not — the same caveat [ADR-0205](0205-environment-parity.md) carries for the local tier.
- **Scenario rot.** A scenario driving `/api/orders` breaks when that contract changes and is not on the per-PR path. Mitigated by label-gated `perf:smoke` and the nightly run.
- **JavaScript reappears outside the sanctioned island.** Bounded by the no-Node, no-npm, no-lockfile rule and by keeping `test/perf/` out of the Bun workspace.

## Rules

- Playwright (TypeScript) is the only browser e2e and visual-regression tool. Cypress, WebdriverIO, Selenium, and pure-Go browser libraries are not used.
- All e2e and visual tests live in the repo-root `test/e2e/` workspace under one Playwright config.
- The browser acceptance test is the platform's acceptance gauge; operator dashboards are tested rendered behind a real AAL2 session, not by HTTP status alone.
- Preflight readiness checks run before the browser suite as failure localisers; they are not acceptance tests.
- E2e runs against `cluster:full` with real services. MSW and all mocking are forbidden in e2e, including the development API mock and the `edge` profile ([ADR-0600](0600-local-development-loop.md)).
- Service integration tests run against `cluster:base` plus the service's declared components and drive services through their generated SDK clients; they do not import another service's code.
- Visual regression gates on committed `toHaveScreenshot` baselines; an intentional UI change updates the baseline in the same PR. Automated rendered-versus-Figma diffing is not a CI gate.
- E2e provisions a committed deterministic test identity — AAL1 user plus AAL2 operator. No test relies on hand-created state.
- Node is permitted solely as the Playwright runner, pinned in `test/e2e/.mise.toml` against the root `[env] NODE_VERSION`, never in the root toolchain. `(CI: lint:node-scope)`
- k6 is the only load-generation tool. Locust, Gatling, JMeter, Vegeta, and hand-rolled generators are not used.
- Performance tests live in `test/perf/` as committed JavaScript, never a GUI-authored or recorded plan.
- `test/perf/` contains no `package.json`, no npm lockfile, and no `node_modules`. `(CI: lint:node-scope)`
- The k6 binary is pinned in `test/perf/.mise.toml`, never in the root `[tools]`.
- Load metrics reach Prometheus as an OTLP push through the OTel collector. A private metrics store, exporter sidecar, or remote-write receiver is not introduced.
- Load series are namespaced `k6_*` and carry `service_name="k6"`; k6's `url`, `name`, and `error` tags are not exported, and request identity is a bounded `endpoint` route-template tag.
- k6 runs with `--no-usage-report`.
- Every scenario declares thresholds and the exit code is the verdict. A scenario without thresholds is a defect.
- Load shapes are `PROFILE`-selected data, not duplicated scenario files.
- Scenarios drive the edge, not port-forwarded pods, so the gateway and forward-auth hop are inside the measurement.
- Performance suites are not part of `mise run test`, `ci:affected`, or the e2e suites, and never implicitly gate a merge.
- Results from a co-hosted generator are reported as relative regression signals, never absolute capacity figures.
- Load against a shared environment requires an explicit target; `test/perf/` defaults to nothing but the local edge.
- The full e2e and perf suites run nightly (activity-gated) and pre-release; smoke suites run per-PR only when labelled. Neither is part of `ci:affected`.
