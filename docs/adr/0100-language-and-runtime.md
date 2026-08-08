# ADR-0100: Language & Runtime

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0101](0101-monorepo.md), [ADR-0104](0104-supply-chain-security.md), [ADR-0204](0204-resource-management.md), [ADR-0302](0302-temporal.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0400](0400-frontend.md), [ADR-0401](0401-internal-admin.md), [ADR-0500](0500-observability.md), [ADR-0600](0600-local-development-loop.md), [ADR-0601](0601-testing-strategy.md)

## Context

The axis position set by [ADR-0000](0000-platform-foundations.md) — decomposition pressure high, operational sovereignty maximal, correctness stakes high — decides this. The product domain does not: two systems at the same axis position get the same answer here whatever they sell, and two systems in the same market routinely sit at opposite ends of every axis.

This ADR picks the **primary backend language**, the **conditions under which a second language is admitted**, and the **language and runtime the frontend is authored in**. The frontend framework is [ADR-0400](0400-frontend.md).

## Decision drivers

1. **The language of the infrastructure being operated.** Axis B at maximum means running, patching, and extending the infrastructure rather than filing a support ticket against it ([ADR-0000](0000-platform-foundations.md), principle 3). The orchestrator and its controller ecosystem, the GitOps controller, the edge proxy, the identity stack, the workflow engine, the telemetry collector, and the metric, log, and trace stores are all one language's output — a property of the era that produced them, not of any selection made here.
2. **Per-service cost against a fixed platform budget.** Axis A high: runtime, toolchain, base image, CI cache, and lint configuration are paid per service and per replica. A cost tolerable once is not tolerable across a fleet.
3. **Correctness affordances at axis C high.** Exact arithmetic, predictable concurrency, and types that refuse to represent an invalid state.
4. **A floor under the two SDKs every service links.** Every service is a durable-execution worker ([ADR-0302](0302-temporal.md)) and a telemetry producer ([ADR-0500](0500-observability.md)). Bindings that are community-maintained or pre-GA put a load-bearing path in the hands of one maintainer. **This admits or excludes a language; it does not rank the languages that clear it.**
5. **One language by default.** A second general-purpose backend language doubles the toolchain, codegen pipeline, CI cache, base image, lint configuration, and review pool. Only a capability the primary language cannot provide justifies it; preference does not.

## Considered options

| Option | Language of the operated stack | Axis C affordances | SDK floor | Verdict |
| --- | --- | --- | --- | --- |
| **Go** | **the stack itself** — Kubernetes and its controller ecosystem, ArgoCD, Traefik, Ory, Temporal's server, the OTel Collector, Prometheus, Loki, and Tempo | static types and CSP concurrency. **No decimal type in the standard library**, and no sum types | clears it, as four others do. Its OTel log signal is beta where .NET's and Java's are stable | **Chosen** on drivers 1 and 5. Verbose error handling and a thin type system are accepted; the arithmetic gap is closed below |
| Rust | no | the strongest on offer — exhaustive matching, no data races by construction, and [`sqlx`](https://github.com/launchbadge/sqlx) checks every query against the live schema at compile time where `sqlc` infers it | **fails it** — Temporal is [first-party at public preview](https://temporal.io/changelog/rust-sdk-public-preview) rather than GA, and [OTel Rust](https://opentelemetry.io/docs/languages/) is beta on all three signals | Best correctness in the field, and driver 4 is unmet while the velocity cost is paid per service. **Kept as an escape hatch** |
| .NET (C#) | no | native 128-bit `decimal`, records, pattern matching — stronger than Go | clears it. [OTel .NET](https://opentelemetry.io/docs/languages/) is stable across all three signals | Loses on driver 1 alone. What it adds over Go does not offset a second operating language |
| JVM (Java/Kotlin) | no | `BigDecimal`; Kotlin adds sealed hierarchies and null tracking | clears it, on the most mature durable-execution SDK in the field | As .NET, plus a build tool and an application framework this ADR would have to choose, and native compilation is a second build mode under its own licence |
| TypeScript on Bun | no | discriminated unions are the best ergonomics here, and types are erased at runtime while numbers are IEEE-754 doubles | clears it | One language across the stack is the appeal. A single-threaded event loop and erased runtime types are the wrong default at axis C high. **Retained for the frontend**, where it is unavoidable |
| Python | no | native `decimal`; annotations are unenforced at runtime | clears it | **Kept as an escape hatch** where the library ecosystem is the reason the service exists |
| Elixir / BEAM | no | supervision trees and true process isolation; dynamically typed | **fails it** — no first-party durable-execution SDK | Its fault-tolerance model overlaps what Temporal and Kubernetes already provide here, so the platform pays for that property twice |
| Polyglot by team choice | mixed | mixed | mixed | The honest baseline. At axis A high the per-language cost multiplies by the number of teams rather than by services, and every shared library is written once per language |

**Runtime footprint no longer separates the compiled options.** [.NET Native AOT](https://learn.microsoft.com/en-us/dotnet/core/deploying/native-aot/) and GraalVM native images put C# and Java in the same order of magnitude as a Go binary at idle and at startup. A decision made on resident memory would once have been decisive and is not, so drivers 1 and 3 carry it instead.

## Decision

| Concern | Decision |
| --- | --- |
| Primary backend language | **Go**, latest stable major |
| Frontend language | **TypeScript** |
| JS runtime for code we author | **Bun**, sole — install, workspaces, test, build, and the production server |
| Escape hatch: Rust | a measured CPU or latency requirement Go does not meet, or a domain whose canonical implementation exists only in Rust |
| Escape hatch: Python | the library ecosystem is the reason the service exists, and the service is a thin wrapper over it |
| A second general-purpose backend language | not adopted |

### Go is chosen for what the platform is made of

At axis B maximal the backend language and the infrastructure language are the same language, because operating infrastructure at that position means reading its source. An admission webhook that panics, a worker's task-queue behaviour under load, an edge rule that matches nothing, a custom collector processor, a controller for a first-party CRD — each is Go, and each is a debugging session or a patch rather than a support case.

**This is the one driver that does not weaken as other runtimes improve**, and it is the driver that flips at a different axis position: a platform at axis B low operates none of that stack and should decide on driver 3, where C# or Kotlin wins. The answer here is a consequence of the position, not a claim about the language.

### Where Go is weak, and what carries the weight

Three gaps are real at axis C high, and none is closed by the language.

| Gap | What it costs | What carries the weight |
| --- | --- | --- |
| No decimal type in the standard library | a `float64` monetary amount compiles and surfaces later as a rounding discrepancy | one shared money type in `libs/go/money/`, over `math/big` in the standard library. A monetary value is that type — at rest, in the contract, and on the wire |
| No sum types, so an invalid state is representable | an illegal state transition compiles | state that must not go invalid is a workflow, where Temporal owns the transitions ([ADR-0302](0302-temporal.md)) |
| Errors are unchecked values | an ignored error compiles | `errcheck` in `golangci-lint` |

`math/big` supplies the arithmetic and none of what makes money correct: a currency that cannot be added to another, a rounding mode chosen rather than inherited, a `numeric` column mapping, and a **string** JSON form. A JSON number is an IEEE-754 double by the time a TypeScript client reads it, so the wire form is a string whatever the Go type is. Those four properties are the type; the arithmetic is the standard library's, and no third-party decimal package is introduced.

The platform does not pretend Go is the strongest instrument for driver 3. It is the strongest for driver 1, and driver 3's gaps have specific mitigations while driver 1's have none.

### What would change this decision

Driver 1 rests on a stack this ADR does not choose, and driver 4 is a floor rather than a ranking. Neither is a vote for the tools named in them.

| Change | Effect |
| --- | --- |
| The workflow engine is replaced | **None.** It is one dependency inside a service, and the floor screens its replacement identically |
| The observability stack is replaced | **None.** Instrumentation is vendor-neutral by [ADR-0500](0500-observability.md)'s own driver, and the backends sit behind it |
| The identity stack or the edge proxy is replaced | **None.** Both have credible equivalents in other languages, rejected on configuration-as-code and licence grounds rather than on language |
| The position on axis B moves down | **Decisive.** The operated stack dissolves, driver 1 evaporates, and driver 3 decides instead |

The orchestration and controller layer is what makes driver 1 robust rather than incidental. Identity could have been a JVM product and the telemetry stores could have been, and those substitutions leave the decision intact. At axis B maximal there is no non-Go orchestrator that is a real option, and that layer is where the custom controllers and the patches are written.

### The JS runtime

| Option | Node API compatibility | Toolchain components | Verdict |
| --- | --- | --- | --- |
| **Bun** | **the highest of the three** | **one binary** — runtime, package manager, workspaces, test runner, and bundler | **Chosen.** Driver 2 applied to the toolchain itself |
| Node.js | the definition of it | a package manager, a test runner, and a bundler selected, pinned, cached, and upgraded separately | Loses on driver 2. Each addition is a version to pin and a supply-chain edge ([ADR-0104](0104-supply-chain-security.md)) |
| Deno | lowest, reached through an `npm:` specifier | comparable to Bun | A second permissions model and a second registry, for no capability this platform needs |

The frontend framework's own vendor ships first-party Bun support, while the framework's self-hosting documentation does not cover the runtime ([vercel/next.js#55272](https://github.com/vercel/next.js/discussions/55272)). That gap is covered by end-to-end tests running against the built image ([ADR-0601](0601-testing-strategy.md)), not by upstream documentation.

### The three Node islands

Node runs vendored third-party tools, never code we author and never a backend runtime. There are exactly three, none in the root toolchain, so an engineer who touches none of them never installs Node. They are the price of the Bun decision, enumerated rather than discovered.

| Island | Why Node is unavoidable | Containment |
| --- | --- | --- |
| Playwright e2e and visual runner ([ADR-0601](0601-testing-strategy.md)) | browser-process control needs extra-fd pipe transport and worker IPC that Bun does not match, and the gap is upstream rather than in our usage — Bun's own author's [support patch](https://github.com/microsoft/playwright/pull/28875) and the [feature request](https://github.com/microsoft/playwright/issues/38095) are both unmerged | `e2e/.mise.toml`, runner and CI only |
| Lowdefy admin console ([ADR-0401](0401-internal-admin.md)) | installed and run rather than built, so the runtime is upstream's choice. Its CLI aborts without `pnpm` on `PATH`, and `lowdefy start` shells to `next start`, whose bin is `#!/usr/bin/env node`. Bun displaces that only by aliasing `node`, and still needs pnpm | `apps/admin/.mise.toml` and that app's image |
| API mock ([ADR-0600](0600-local-development-loop.md)) | the only OpenAPI-3.1-complete contract mock is a Node program | consumed as a pinned upstream container, so it installs no Node anywhere and adds no `package.json` |

### What makes a language an escape hatch

An escape hatch adds a capability Go lacks. A language that does what Go already does is a **substitute**: adopting one buys nothing and pays driver 5 in full. That is why JVM, .NET, and a JavaScript backend are barred with or without an ADR, each being stronger than Go on driver 3 and none adding a capability.

Rust and Python are complements rather than substitutes, and each is admitted only on the condition in the decision table. Correctness preference is driver 3, and driver 3 does not outrank driver 5 on its own.

Every escape-hatch service carries its own ADR recording the measured need, and a Rust service additionally records how it satisfies [ADR-0500](0500-observability.md) given the maturity of its telemetry SDKs. A third escape hatch amends this ADR.

## Consequences

### Positive

- One language across the fleet: `libs/go/` is shared unconditionally, with one lint and format configuration, one base image, and one codegen pipeline. An engineer moves between services without a toolchain.
- **The platform and the product are written in the same language.** A stack trace out of a third-party component is readable, and a fix to one is an ordinary pull request rather than an upstream issue and a wait.
- A service's resident memory is dominated by its own working set rather than by its runtime, so fleet capacity tracks workload ([ADR-0204](0204-resource-management.md)).
- Generated TypeScript clients give cross-language type sharing without a second backend language ([ADR-0303](0303-api-contracts-and-lifecycle.md)).
- The decision is derived from the axis position, so a project adopting this template at a different position knows which driver to re-run rather than having to re-argue the language.

### Negative / Risks

- **Go is not the strongest available language on driver 3**, and this ADR knowingly ranks driver 1 above it. Accepted, on the argument above.
- Verbose error handling is accepted. No bespoke error-handling DSLs.
- **The money type is first-party code on a correctness-critical path.** A bug in it is a bug in every price, total, and ledger entry at once, where a language with a native decimal inherits a vetted implementation instead. It carries no third-party dependency, so the exposure is ours to test rather than ours to trust.
- **Bun is a single-vendor runtime with one implementation.** The exit is affordable only because its Node API compatibility is high enough to run the same code on Node, which the islands already demonstrate is installable.
- Every escape-hatch service is a permanent tax: a toolchain, codegen pipeline, CI cache, base image, and review pool that exist for one service.

## Rules

- Every backend service is written in Go. Another language requires its own ADR admitting the service under a documented escape hatch. `(review-only)`
- The Go version is pinned by the root `.mise.toml` and no service overrides it. Only a release still receiving upstream security fixes is pinned — the two most recent major releases ([Go release policy](https://go.dev/doc/devel/release)). `(CI: ci-lint)`
- A monetary amount is the shared money type, never a floating-point number — `numeric` in the database, a string in the OpenAPI schema, and a string on the wire. `(review-only)`
- The frontend is TypeScript. `(review-only)`
- Bun is the only JS runtime for code we author. No Go service image, the frontend image, or any artifact built from our own source installs Node. `(CI: lint:node-scope)`
- Node is never pinned in the root `.mise.toml`. An island that installs Node pins it in its own island config against the root `[env] NODE_VERSION`; an island shipping as a container pins no Node at all. A fourth Node island requires amending this ADR. `(CI: lint:node-scope)`
- A Rust service requires its own ADR recording measured Go inadequacy or a Rust-native canonical ecosystem, and how the service satisfies [ADR-0500](0500-observability.md). `(review-only)`
- A Python service requires its own ADR and is admitted only where the library ecosystem is the reason the service exists. `(review-only)`
- JVM, .NET, and JavaScript backends are not permitted, with or without an ADR. `(review-only)`
- Cross-language sharing happens through generated OpenAPI clients, never a shared in-process runtime. `(review-only)`
