# ADR-0101: Monorepo Structure & Build

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0100](0100-language-and-runtime.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0401](0401-internal-admin.md), [ADR-0601](0601-testing-strategy.md)

## Context

One repository hosts the whole fleet of backend services, one frontend application, shared Go and TypeScript libraries, generated API clients, infrastructure-as-code, and tooling. Every engineer touches it, and "every PR runs every test in every package" does not survive past roughly 20 services.

This ADR answers six questions in one place: repository layout, task invocation, workspace tooling, CI, codegen, and frontend topology.

## Decision drivers

1. **One task entry point per concern.** `mise run test` does the right thing in any directory.
2. **Native caches first.** Go's build and test cache, Docker BuildKit, Bun's install cache. Higher-level orchestration is added only when these are insufficient.
3. **Affected detection is mandatory.** Running every test on every PR is acceptable only up to about 10 services.
4. **Local and CI run the same commands.** No CI-only shell scripts.
5. **Build-graph tools are an upgrade path, not a starting point.** Their value is dominated by team size, which is the binding constraint ([ADR-0000](0000-platform-foundations.md)).

## Considered options

### Task runner

| Option | Multi-language | Pins tool versions | Verdict |
| --- | --- | --- | --- |
| **mise** | yes | **yes — the same file** | **Chosen.** One tool for both concerns, so a task and the toolchain it needs cannot drift apart |
| Make | yes | no | Ubiquitous, and tool versions become a separate unpinned problem |
| just | yes | no | Better ergonomics than Make, same gap |
| Nx | TypeScript-first | no | A build graph with caching, whose value needs more apps than this repo has. The documented upgrade path |
| Bazel | yes | yes, hermetically | Reserved for when reproducible hermetic builds become a compliance requirement, not before |

### Go module strategy

| Option | Dependency consistency | Cross-cutting refactor | Verdict |
| --- | --- | --- | --- |
| **Single repo-wide `go.mod`** | **structural — drift is impossible** | one PR | **Chosen** |
| Module per service | isolated | staggered bumps across N modules | Buys isolation the platform does not need and costs consistency it does |
| `go.work` overlay | partial | workspace-mode ceremony | The complexity of multi-module without the isolation benefit |

### JavaScript package manager

| Option | Verdict |
| --- | --- |
| **Bun** | **Chosen** — faster install, fewer flags, and it is already the runtime ([ADR-0100](0100-language-and-runtime.md)). Lockfile `bun.lockb`, committed |
| pnpm, npm | A second JS toolchain beside the runtime, for no capability gained |
| Turborepo | Its value needs multiple frontend apps. Re-evaluated only if the frontend grows, which itself requires an ADR |

## Repository layout

```text
go.mod                        # single Go module for the entire repo
go.sum

services/<name>/              # one backend service (package, not a module)
├── openapi.yaml
├── cmd/{server,worker}/
├── internal/{handlers,workflows,activities,domain,store}/
├── migrations/
├── Dockerfile
└── .mise.toml

apps/
├── frontend/                 # one Next.js app, route groups inside
│   └── src/app/(landing|panel|devportal)/
└── admin/                    # Lowdefy config for internal admin (ADR-0401)
    ├── lowdefy.yaml
    ├── _generated/<service>/ # codegen from OpenAPI, drift-checked
    └── custom/<service>/     # hand-written pages

libs/
├── go/                       # all shared Go (single go.mod, no per-library module)
│   ├── <name>/
│   └── sdks/<service>/       # generated Go server + client
└── ts/                       # all shared TS (workspace root)
    ├── <name>/
    └── sdks/<service>/       # generated TS client

infra/
├── terraform/                # cluster + DNS + LB provisioning
├── helm/                     # Helm charts (ours + values for upstream)
├── gitops/                   # ArgoCD ApplicationSets + per-env values
├── ansible/                  # host configuration
├── auth/                     # Kratos, Hydra, OpenFGA config
├── gateway/                  # Traefik routing + rate limits
└── observability/            # dashboards and alerts as code

tools/                        # repo-local Go programs
docs/                         # ADRs, runbooks, conventions
.github/workflows/            # CI definitions
```

| Choice | Reason |
| --- | --- |
| `services/` and `apps/` are siblings | Services are headless, horizontally-scaled, internally-addressed backends. `apps/` holds first-party deployable applications, and the slot stays open for partner portals, CLIs, or mobile apps without a layout migration. A new entry under `apps/` requires its own ADR |
| `services/` stays spelled out | `svc` already means a Kubernetes Service here and is the per-service metavariable in docs. Directory names abbreviate only when unambiguous |
| `infra/` holds both IaC and the configuration of what it deploys | One top-level directory removes the recurring "infra or ops?" question |
| `tools/` holds repo-local Go programs only | It is not a shell-script drawer. External tools are installed by mise |

## Decision

### mise is the single entry point

Tasks live in `.mise.toml` files: a root file for repo-wide tasks, one per service for service-local tasks. `mise tasks --list` is the discoverable interface.

| Scope | Standard task names |
| --- | --- |
| Every service | `build`, `test`, `lint`, `generate`, `migrate`, `server`, `worker` |
| Repo root | `cluster:base`, `cluster:stop`, `ci:lint`, `ci:test`, `ci:build`, `ci:affected`, `ci:gen`, `e2e`, `e2e:smoke`, `gen`, `db:migrate` |

The two long-running service tasks are named for the process type they start, matching `cmd/{server,worker}/`, the `<service>-{server,worker}` images, and the in-cluster DNS names — one vocabulary end to end. A frontend is not a service and keeps `run`: it has no worker to be symmetric with, and the task starts a dev server rather than a production binary.

**Task naming.** A task name is `group:member`, where the group is the axis you want to list and run together. Two shapes are correct, and which one applies is decided by asking what you would browse or aggregate by:

| Shape | Use when | Examples |
| --- | --- | --- |
| `activity:target` | one activity fans out across many targets, with an umbrella task | `lint:go`, `lint:ts`, `lint:md` under `lint`; `format:*`; `gen:openapi`, `gen:sqlc` under `gen` |
| `resource:operation` | a stateful thing has a lifecycle worth grouping | `cluster:ensure`/`stop`/`delete`, `service:deploy`/`undeploy`, `db:migrate`, `ops:grant` |

Forcing one shape on both scatters a family: `stop:cluster` and `delete:cluster` split the cluster lifecycle, and `ts:format` breaks the `format` umbrella. Use `activity:` only where a real umbrella exists. Graph-only plumbing that exists solely as a `depends` node is marked `hide = true`.

### Every executable is pinned, in one of two places

| Kind | Pinned in |
| --- | --- |
| Developer and CI tools — Go, Bun, `sqlc`, `sqruff`, `ogen`, `vacuum`, `helm`, `kubectl`, `age`, `sops`, mise itself | the root `.mise.toml`, or a service-local one where a service genuinely needs a different version |
| Runtime services — Postgres, Temporal, Kratos, Oathkeeper, OpenFGA, MinIO, the observability stack, Argo CD, the CNPG operator | Helm chart `appVersion` plus an explicit `image.tag` in `infra/helm/.../values.yaml` |

Floating tags — `latest`, `stable`, `main`, an unpinned major — are forbidden in `.mise.toml`, Dockerfiles, Helm values, and workflows. A tool-version change is a normal PR, so anyone cloning at any SHA reproduces the exact toolchain that built it.

### One Go module

One `go.mod` at the repo root covers every service, library, generated client, and `tools/` program. No `go.work`, no per-service module, no `replace` directives. Services and libraries are plain packages, imported by the repo's module path.

Dependency **consistency** is worth more here than dependency **isolation**: one `go get -u` upgrades the repo, one `govulncheck` and one `go.sum` describe it, cross-cutting refactors land in one PR, and every service is structurally forced onto the same version of every dependency.

The accepted costs are in *Consequences*.

### One Next.js app

The frontend is one application with route groups for `(landing)`, `(panel)`, and `(devportal)`.

| Reason | Detail |
| --- | --- |
| Cross-subdomain auth is hostile | Sharing cookies and session across subdomains is a recurring source of subtle production bugs, Safari ITP especially |
| Bundle size is not the constraint | Next.js route-level code splitting weakens the separate-apps argument |
| One deploy unit | One Traefik routing surface: `/panel/*` as a path, not a hostname |

A genuinely independent frontend — a partner-branded experience, an embedded SDK — earns its own ADR. It is not a reason to fragment the primary app pre-emptively.

Bun workspaces unify the app and TS libraries:

```jsonc
// package.json (root)
{ "workspaces": ["apps/frontend", "libs/ts/*", "libs/ts/sdks/*"] }
```

### CI

| Workflow | Runs |
| --- | --- |
| `lint.yml`, `test.yml`, `build.yml` | `mise run ci:lint` / `ci:test` / `ci:build` |
| `ci-drift.yml` | `mise run ci:gen`, failing on `git diff --exit-code` |
| `publish.yml` | builds and pushes images on merges to `master` |
| `e2e.yml` | nightly and pre-release full suite, plus a label-gated smoke job ([ADR-0601](0601-testing-strategy.md)) |

Lint, test, and build are **whole-repo over the single Go module**, not affected-scoped: the linters have no per-service subset to take, and Go's build cache makes the repeat cost small. `ci:affected` scopes what gets built and promoted, not what gets analysed.

Every workflow takes its toolchain from the `./.github/actions/setup` composite action, so mise setup and the Go cache key are defined once. Which forge runs these workflows, and on whose runners, is [ADR-0102](0102-source-control-and-ci.md); the thin-YAML rule there is what keeps that choice reversible.

### Affected detection

`tools/affected/` reads `git diff --name-only origin/master...HEAD` and maps changes to scopes:

| Change under | Affects |
| --- | --- |
| `services/<X>/` | service `<X>` |
| `libs/go/<L>/` | every Go consumer of `<L>`, via `go list -deps` |
| `libs/go/sdks/<S>/` | every Go consumer of service `<S>`'s client |
| `apps/frontend/` | the frontend |
| `infra/`, `tools/`, `go.mod`, `go.sum`, `package.json` | **global** — everything runs |

`mise run ci:affected` produces a JSON manifest the workflows consume. This is the most load-bearing piece of repo tooling as the fleet grows, and it carries unit tests.

### Codegen is committed and drift-checked

Generated code is committed: `libs/{go,ts}/sdks/<service>/` for OpenAPI clients, and `services/<service>/internal/store/` for sqlc output. PR diffs then include the generated changes, `go build` works without a codegen step, and CI stays simple.

`mise run ci:gen` regenerates everything and CI fails on a diff. A lefthook pre-commit hook runs the relevant slice when source files change.

### Caching

| Cache | Keyed on |
| --- | --- |
| Go build and test | the root `go.sum` |
| Docker BuildKit layers | the registry |
| Bun install | `bun.lockb` |

**Deferred:** a remote Go build cache (`GOCACHEPROG`, `sccache`). **Trigger:** CI time consistently exceeding 15 minutes on the median affected PR. **Seam:** it is a `GOCACHEPROG` environment variable, so adoption is configuration.

### Container images

One multi-stage `Dockerfile` per service: stage 1 builds with the workspace `go build`, stage 2 is `gcr.io/distroless/static-debian12`. One Dockerfile for the frontend, using Next.js standalone output. There is no shared base image; distroless static is sufficient.

**Server and worker are separate images from the same Dockerfile.** A `CMD` build arg selects which `cmd/` binary the build stage compiles, producing `<service>-server:<sha>` and `<service>-worker:<sha>`. They scale independently, have different resource profiles, and should not carry each other's code surface; one Dockerfile keeps the build stage and base layers shared and cache-friendly.

Images are tagged `<service>:<git-sha>`, and the same SHA flows through every environment ([ADR-0201](0201-gitops.md)).

### Upgrade path

The day-one stack is sized for roughly 30–50 services with disciplined dependencies. Each step below is its own ADR when triggered.

| Step | Trigger |
| --- | --- |
| Split the Go module | A service or library genuinely needs its own dependency line — external publication, divergent upgrade cadence, isolated security review |
| Adopt Nx as a multi-language task orchestrator with caching | The task graph outgrows mise, with mise still managing tool versions |
| Adopt Bazel | Hermetic reproducible builds become a compliance requirement |

## Consequences

### Positive

- `mise run <task>` is the universal interface across languages and services.
- Cross-package Go changes land in one PR with no `replace` directive or workspace ceremony.
- Every service is structurally forced onto the same dependency versions — no drift, no per-service upgrade backlog, and one Renovate PR per dependency rather than N.
- One frontend app removes the entire class of cross-subdomain auth bugs.
- Committed codegen keeps PR review honest and CI simple.
- Affected detection ties CI runtime to PR size rather than repo size.

### Negative / Risks

- **Affected detection is custom code**, and a bug produces "tests passed but the affected service was broken". Mitigated by unit tests, an explicit `--all` fallback, and global-trigger paths that promote any shared-infrastructure change to a full run.
- **A single `go.mod` means a service cannot pin its own version of a shared dependency.** A risky upgrade lands repo-wide or not at all. Mitigated by `depguard` rules on sensitive packages and by the documented module-split path.
- **A single `go.mod` couples vulnerability blast radius**: a CVE in any transitive dependency flags the whole repo. Mitigated by treating Go upgrades as routine repo-wide work.
- **Extracting a service to its own repo is a real migration**, not a directory move. Accepted — this is an internal product, not a library ecosystem.
- **One frontend app risks becoming a god-app.** Mitigated by lint-enforced route-group boundaries and the ADR-required path for genuinely independent frontends.
- **Committed generated code inflates repo size.** Mitigated by routine `git gc` and `git repack`.
- **A single `go build ./...` compiles more than one service needs.** The native build cache absorbs it, and affected detection keeps CI proportional to PR size.

### Follow-ups

- `tools/affected/` with unit tests.
- Root `.mise.toml`, `package.json`, and `go.mod`.
- `services/_template/` service skeleton.
- `.github/workflows/{lint,test,build,ci-drift,publish,promote-on-merge,promote-on-release,e2e}.yml`.
- `depguard` rule preventing `services/<X>/` from importing `services/<Y>/`.
- Lint rule preventing cross-route-group imports in `apps/frontend/`.
- Renovate config for the single Go module.

## Rules

- The repo is a single Go module rooted at `go.mod`. There are no per-service or per-library `go.mod` files and no `go.work`. `(CI: ci-lint)`
- Every backend service lives at `services/<name>/`; every shared Go package under `libs/go/<name>/`; every shared TypeScript library under `libs/ts/<name>/`. `(CI: lint:service-contract)`
- Generated API clients live at `libs/{go,ts}/sdks/<service>/` and are committed. `(CI: ci-drift)`
- The frontend is one Next.js app at `apps/frontend/`. A new frontend or a new entry under `apps/` requires an ADR. `(review-only)`
- Tasks are invoked through `mise run <task>`. Every service exposes `build`, `test`, `lint`, `generate`, `migrate`, `server`, `worker`. `(CI: lint:service-contract)`
- A task name is `group:member`, grouped by the axis worth listing together. `(review-only)`
- Every external tool is pinned: developer and CI tools in `.mise.toml`, runtime services as an explicit `image.tag` in Helm values. Floating tags are not used anywhere. `(CI: lint:floating-tags)`
- A PR changing a spec, SQL query, or any codegen input includes the regenerated artifacts. `(CI: ci-drift)`
- A change to `go.mod`, `go.sum`, root `package.json`, `infra/`, or `tools/` triggers a full-repo CI run. `(CI: ci-affected)`
- Container images are tagged `<service>:<git-sha>`, and the same SHA flows through every environment. `(CI: lint:floating-tags)`
- `services/<X>/` does not import `services/<Y>/`. Sharing happens through `libs/` or generated clients. `(CI: ci-lint)`
- Route groups inside `apps/frontend/` do not import from each other. `(CI: ci-lint)`
- Build-graph tools are not used on day one. Adoption requires its own ADR. `(review-only)`
