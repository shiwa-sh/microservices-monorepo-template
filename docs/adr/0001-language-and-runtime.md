# ADR-0001: Language & Runtime

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md)

## Context

At the target scale ([ADR-0000](0000-platform-foundations.md)), workloads span CRUD APIs, payments (financial-grade correctness), blockchain integration, and occasional high-throughput paths. The frontend is Next.js.

We pick:

1. The **primary backend language** that every service uses unless an ADR sanctions otherwise.
2. **Sanctioned escape hatches** for cases where the primary language is the wrong tool.
3. The **frontend language** (effectively decided by Next.js).

## Decision drivers

In priority order:

1. **Per-service cost.** 100 services × 8 engineers means fat runtimes, slow builds, and heavy frameworks compound 100×.
2. **Cloud-native ecosystem fit.** Kubernetes, OpenTelemetry, Temporal, OpenAPI codegen, sqlc — first-class libraries, not bolt-ons.
3. **Correctness for financial / blockchain workloads.** Strong typing, predictable concurrency, no surprises with money or chain state.
4. **Hiring + onboarding.** Popular enough to hire for; simple enough to onboard in weeks, not months.
5. **Single language by default.** Polyglot is allowed only with explicit ADR-level justification per service.

## Considered options

- **Go** — best fit for our stack. Fast cold start (~100 ms), small memory footprint (~30 MB idle), first-class SDKs for Temporal, OTel, k8s, sqlc, ogen. Verbose error handling and less expressive type system are accepted costs.
- **Rust** — best correctness and performance; velocity cost is real at 100-service scale with a small team; Temporal SDK is community-maintained. Kept as an escape hatch.
- **JVM (Java/Kotlin)** — mature, but per-service memory footprint (200–500 MB) and JVM tuning tax are incompatible with the 100-service target.
- **Node.js / TypeScript backend** — shared language with frontend is appealing, but single-threaded event loop and erased runtime types are wrong for CPU-bound and financial workloads.
- **.NET (C#)** — competitive runtime, but cloud-native ecosystem (Temporal, OTel, k8s) is consistently one step behind Go.
- **Python** — wrong tool for typed, high-throughput services. Kept as an escape hatch for ML/data work where the Python ecosystem *is* the reason for the service.

## Decision

- **Primary backend language: Go.** Latest stable major version, tracked in `.mise.toml`.
- **Frontend language: TypeScript** via Next.js, latest stable. **Bun is the sole JS runtime** — install, dev, build, and the production `server.js` all run under Bun. Node.js is not installed anywhere.
- **Sanctioned escape hatches:**
  - **Rust** for services with measured CPU/latency requirements Go cannot meet, or for blockchain components whose canonical libraries are Rust-native.
  - **Python** for ML/data services where the Python scientific ecosystem is the reason the service exists. Never permitted for general API services.
- Every escape-hatch service requires its own ADR documenting the measured need.

**Rejected as primary:** Rust (velocity), JVM (footprint), Node.js backend (concurrency, type-safety), .NET (ecosystem gap), Python (wrong tool).

**Rejected as escape hatch:** JVM, .NET, Node.js backend — adding them solves no problem Go cannot, and each doubles ops surface.

## Consequences

### Positive

- One language across ~100 services. Shared libraries in `libs/go/`, shared lint/format, easy engineer rotation.
- Predictable per-service footprint (~30 MB image, ~30 MB idle RAM) keeps 100 services tractable on the cluster sizes chosen in [ADR-0003](0003-cluster-topology.md).
- Tightest fit with everything else we use: Temporal, OTel, Kubernetes, sqlc, ogen are all Go-native.
- Frontend TS + generated TS clients gives cross-language type sharing without polyglotting the backend.

### Negative / Risks

- Go's type system is less expressive than Rust or Kotlin. Discipline + linters (`golangci-lint`) + codegen (sqlc, ogen) compensate.
- Verbose error handling is accepted. No bespoke error-handling DSLs.
- Every Rust or Python service is a permanent ops tax: separate toolchain, separate codegen pipeline, separate CI cache, separate hire profile. The ADR requirement makes adoption deliberate.

### Follow-ups

- `.golangci.yml` at repo root with rules referenced from other ADRs.
- An ADR-template for escape-hatch services in `docs/adr/_template-escape-hatch.md`.

## Rules

- Every backend service is written in Go unless an ADR sanctions an escape hatch.
- Go version is pinned by `.mise.toml`; services do not override it.
- The frontend is TypeScript on Next.js, single app per [ADR-0002](0002-monorepo.md).
- Bun is the only JS runtime — Node.js is not installed locally or in any container image.
- A Rust service requires its own ADR demonstrating measured Go inadequacy or Rust-native ecosystem need.
- A Python service requires its own ADR; it is permitted only for ML/data workloads.
- JVM, .NET, and Node.js backends are not permitted, with or without an ADR.
- Cross-language sharing happens through OpenAPI clients, never through a shared in-process runtime.
