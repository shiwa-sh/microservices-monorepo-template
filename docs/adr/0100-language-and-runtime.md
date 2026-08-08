# ADR-0100: Language & Runtime

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0101](0101-monorepo.md), [ADR-0401](0401-internal-admin.md), [ADR-0600](0600-local-development-loop.md), [ADR-0601](0601-testing-strategy.md)

## Context

Workloads span CRUD APIs, payments requiring financial-grade correctness, blockchain integration, and occasional high-throughput paths. The frontend is Next.js.

This ADR picks three things: the **primary backend language** every service uses unless an ADR sanctions otherwise, the **sanctioned escape hatches** for cases where that language is the wrong tool, and the **frontend language**.

## Decision drivers

1. **Per-service cost.** Fat runtimes, slow builds, and heavy frameworks are multiplied by the service count against a fixed platform-engineering budget ([ADR-0000](0000-platform-foundations.md)). A cost tolerable once is not tolerable per service.
2. **Cloud-native ecosystem fit.** Kubernetes, OpenTelemetry, Temporal, OpenAPI codegen, and sqlc as first-class libraries rather than bolt-ons.
3. **Correctness for financial and blockchain workloads.** Strong typing, predictable concurrency, no surprises with money or chain state.
4. **Hiring and onboarding.** Popular enough to hire for, simple enough to onboard in weeks.
5. **One language by default.** Polyglot requires ADR-level justification per service.

## Considered options

| Option | Idle footprint | Cold start | Ecosystem fit | Verdict |
| --- | --- | --- | --- | --- |
| **Go** | ~30 MB | ~100 ms | Temporal, OTel, Kubernetes, sqlc, ogen are all Go-native | **Chosen.** Verbose error handling and a less expressive type system are accepted costs |
| Rust | smallest | fastest | Temporal SDK is community-maintained | Best correctness and performance. The velocity cost is real across a fleet against a fixed budget. **Kept as an escape hatch** |
| JVM (Java/Kotlin) | 200–500 MB | slow | mature | The per-service footprint and tuning tax multiply by the service count, which driver 1 forbids |
| Node.js / TypeScript backend | moderate | fast | good | A shared language with the frontend is appealing, and a single-threaded event loop with erased runtime types is wrong for CPU-bound and financial work |
| .NET (C#) | competitive | competitive | consistently one step behind Go for Temporal, OTel, Kubernetes | Rejected on ecosystem lag alone |
| Python | moderate | fast | wrong for typed high-throughput services | **Kept as an escape hatch** for ML and data work where the Python ecosystem *is* the reason the service exists |

**Rejected as an escape hatch:** JVM, .NET, and a Node.js backend. Each solves nothing Go cannot and doubles the operational surface.

## Decision

| Concern | Decision |
| --- | --- |
| Primary backend language | **Go**, latest stable major, pinned in `.mise.toml` |
| Frontend language | **TypeScript** on Next.js, latest stable |
| JS runtime for code we author | **Bun**, sole — install, dev, build, and the production `server.js` |
| Escape hatch: Rust | services with measured CPU or latency requirements Go cannot meet, or blockchain components whose canonical libraries are Rust-native |
| Escape hatch: Python | ML and data services only. Never a general API service |
| Escape hatch: Node.js | solely to run vendored third-party tools that ship as Node programs |

Every escape-hatch service requires its own ADR recording the measured need.

### The three Node islands

Node runs vendored third-party tools, never code we author and never a backend runtime. There are exactly three, none in the root toolchain, so a developer who touches none of them never installs Node.

| Island | Why Node is unavoidable | Containment |
| --- | --- | --- |
| Playwright e2e and visual runner ([ADR-0601](0601-testing-strategy.md)) | Bun cannot reliably run a browser test runner: extra-fd pipe transport and worker IPC are corners of `child_process` Bun has not matched. A Node-ecosystem-wide gap, not a tool that can be swapped to avoid it | `e2e/.mise.toml`, runner and CI only |
| Lowdefy admin console ([ADR-0401](0401-internal-admin.md)) | Installed and run rather than built, so the runtime is upstream's choice. Its CLI aborts without `pnpm` on PATH, and `lowdefy start` shells to `next start`, whose bin is `#!/usr/bin/env node`. Bun cannot displace that without aliasing `node` and still needing pnpm | `apps/admin/.mise.toml` and that app's image |
| API mock ([ADR-0600](0600-local-development-loop.md)) | The only OpenAPI-3.1-complete contract mock is a Node program | Consumed as a pinned upstream container, so it installs no Node anywhere and adds no `package.json` |

## Consequences

### Positive

- One language across the fleet: shared libraries in `libs/go/`, shared lint and format, easy engineer rotation.
- A predictable per-service footprint — roughly 30 MB image and 30 MB idle RAM — keeps the fleet tractable on the cluster sizes [ADR-0200](0200-cluster-topology.md) chooses, and keeps it true as the fleet grows.
- Tightest fit with the rest of the stack, all of which is Go-native.
- Frontend TypeScript plus generated TypeScript clients gives cross-language type sharing without polyglotting the backend.

### Negative / Risks

- Go's type system is less expressive than Rust's or Kotlin's. Compensated by `golangci-lint` and by codegen — sqlc and ogen produce the types that would otherwise be hand-written.
- Verbose error handling is accepted. No bespoke error-handling DSLs.
- Every Rust or Python service is a permanent operational tax: a separate toolchain, codegen pipeline, CI cache, and hire profile. The per-service ADR requirement makes adoption deliberate.

### Follow-ups

- `.golangci.yml` at repo root, with rules referenced from other ADRs.
- `docs/adr/_template-escape-hatch.md` for escape-hatch services.

## Rules

- Every backend service is written in Go unless an ADR sanctions an escape hatch. `(review-only)`
- The Go version is pinned by `.mise.toml`; services do not override it. `(CI: ci-lint)`
- The frontend is TypeScript on Next.js, one app ([ADR-0101](0101-monorepo.md)). `(review-only)`
- Bun is the only JS runtime for code we author. No Go service image, the frontend image, or any artifact built from our own source installs Node. `(CI: lint:node-scope)`
- Node is never pinned in the root `.mise.toml`. An exception that installs Node pins it in its own island config against the root `[env] NODE_VERSION`; an exception shipping as a container pins no Node at all. A fourth Node island requires amending this ADR. `(CI: lint:node-scope)`
- A Rust service requires its own ADR demonstrating measured Go inadequacy or a Rust-native ecosystem need. `(review-only)`
- A Python service requires its own ADR and is permitted only for ML and data workloads. `(review-only)`
- JVM, .NET, and Node.js backends are not permitted, with or without an ADR. `(review-only)`
- Cross-language sharing happens through OpenAPI clients, never a shared in-process runtime. `(review-only)`
