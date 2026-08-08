# ADR-0101: Monorepo Structure & Build

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0100](0100-language-and-runtime.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0401](0401-internal-admin.md), [ADR-0601](0601-testing-strategy.md)

## Context

A fleet of backend services, one frontend application, shared Go and TypeScript libraries, generated API clients, infrastructure-as-code, and tooling have to live somewhere. Whether that is one repository or many is decided here, and everything after it follows from that answer.

This ADR answers seven questions in one place: repository topology, layout, task invocation, workspace tooling, CI, codegen, and frontend topology.

## Decision drivers

1. **One task entry point per concern.** The same command does the right thing in any directory and any language, discoverable without reading CI configuration.
2. **Native caches first.** Go's build and test cache, Docker BuildKit, Bun's install cache. Higher-level orchestration is added only when these are insufficient.
3. **CI cost tracks what changed, not what exists.** A repository holding the whole fleet makes every pull request a potential full-fleet run unless something scopes it, and that cost grows with the repository while the value of the run does not.
4. **Local and CI run the same commands.** No CI-only shell scripts.
5. **Orchestration is bought with measured need.** A build-graph tool's value scales with team and application count, and the binding constraint here is platform-engineering capacity ([ADR-0000](0000-platform-foundations.md)). Its cost is paid from day one against that budget.

## Considered options

### Repository topology

| Option | A change crossing a service boundary | Dependency drift | CI blast radius | Verdict |
| --- | --- | --- | --- | --- |
| **One repository, one Go module** | **one pull request that compiles and tests every consumer** | **structurally impossible** | every change can reach everything, so scoping is mandatory rather than optional | **Chosen** |
| Repository per service | N pull requests across N review queues, merged in an order nothing enforces | the default state — each service pins its own versions and they diverge | naturally scoped | The honest baseline, and what happens when nobody decides. The isolation is real and is the wrong purchase at axis A high with one platform team |
| Hybrid — platform repository, services split out | as repository-per-service for the split parts | as repository-per-service for the split parts | mixed | Two toolchains, two CI shapes, two release paths, and the boundary is relitigated per service |
| One repository of submodules | a submodule bump per repository, plus a superproject bump | as repository-per-service | mixed | The coordination cost of many repositories with the review surface of one |

**The deciding property is the generated client.** Every service publishes a contract that generates clients compiled into other services ([ADR-0303](0303-api-contracts-and-lifecycle.md)). Across a repository boundary that generation is a publish-and-consume cycle, so a breaking contract change becomes a version negotiation between queues; in one repository it is a compile error in the pull request that caused it. At axis A high the number of those boundaries is the thing that grows.

The choice is reversible in one direction only: adding a repository is easy, and extracting a service from this one is a migration. That asymmetry is priced in *Consequences*.

### Task runner

Two questions are bundled here, and the discriminator is that most candidates answer only one: **running tasks** and **pinning the toolchain those tasks need**.

| Option | Runs tasks | Pins tool versions | Verdict |
| --- | --- | --- | --- |
| **mise** | **yes** | **yes, in the same file** | **Chosen.** One tool for both, so a task and the toolchain it needs cannot drift apart |
| Make | yes | no | Ubiquitous, and the toolchain becomes a separate unpinned problem |
| just | yes | no | Better ergonomics than Make, same gap |
| asdf | no | yes | The version manager whose plugin ecosystem mise is compatible with, and it needs a task runner beside it |
| direnv | no | environment variables only | Solves neither question; composes with whatever does |
| Nix — flakes, `devenv`, `devbox` | through flake apps, or a runner beside it | **yes, hermetically, down to system libraries** | The strongest reproducibility on offer, and it answers a question this repository has largely already answered — see below |
| Nx | yes | no | A build graph with caching, whose value needs more applications than this repo has. A documented upgrade path |
| moon, with `proto` | yes | through `proto`, a second binary resolving from a curated registry | The closest thing to a direct competitor, and the only candidate that ships affected detection rather than leaving it to be written — see below. A documented upgrade path |
| Pants | yes | yes, hermetically | Bazel's family, with dependency inference instead of hand-written build files and a real Go backend. Held against the same compliance trigger, where it is the more likely answer than Bazel |
| Bazel | yes | yes, hermetically | Reserved for the compliance trigger below, alongside Pants and Nix |

**On moon specifically.** It answers both questions, and it ships the one capability this ADR otherwise builds by hand: a project graph with affected detection and task caching. *Consequences* records that custom affected detection is this repository's most load-bearing piece of tooling and a bug in it produces a green run against a broken service — moon would retire that code. Three things decide against it now. It is two binaries where mise is one file, since tool pinning lives in `proto` beside it. Its depth is in the JavaScript ecosystem, with other languages arriving as WASM toolchain plugins and Bun a later addition than the Node package managers — the inverse of a repository that is Go-primary with one JavaScript application. And the two pin tools by different architectures: `proto` resolves from a curated registry, its own documentation stating that it is "not possible for proto to support *everything* in core directly", so a tool outside the core toolchain and the community registry is a plugin written here — configuration-based in simple cases, WASM otherwise. mise resolves a tool from its GitHub releases with no plugin and no per-tool entry. That difference is invisible for Go and Bun and decisive for the rest of the pinned set, which is largely single-binary Go releases. Driver 5 applies as it does to Nx: the graph is bought when the need is measured, and the seam is that tasks are already declarative and named `group:member`.

**On Nix specifically.** It pins system libraries rather than only tool versions, which is a real capability nothing else here has. Three things weigh against it. It is not a task runner, so one sits beside it, and driver 1 forbids splitting task invocation across two tools. It puts a second language and a daemon on every workstation and every CI runner, paid from the platform-engineering budget that is the binding constraint ([ADR-0000](0000-platform-foundations.md)). And the gap it closes is the system-library layer — a statically linked Go binary on a distroless base has almost none of that surface, so the payoff here is a fraction of what it is for a dynamically linked polyglot fleet. It returns as a candidate when hermeticity stops being a nicety, which is the same trigger Bazel is held against.

### Go module strategy

| Option | Dependency consistency | Cross-cutting refactor | Verdict |
| --- | --- | --- | --- |
| **Single repo-wide `go.mod`** | **structural — drift is impossible** | one PR | **Chosen** |
| Module per service | isolated | staggered bumps across N modules | Buys isolation the platform does not need and costs consistency it does |
| `go.work` overlay | partial | workspace-mode ceremony | The complexity of multi-module without the isolation benefit |

### JavaScript workspace orchestration

[ADR-0100](0100-language-and-runtime.md) settles the runtime and package manager: Bun, sole, covering install and workspaces. What remains here is whether a separate orchestrator sits above it.

| Option | Verdict |
| --- | --- |
| **Bun workspaces alone** | **Chosen.** The workspace protocol is already inside the selected tool, so the repository adds no JavaScript component |
| Turborepo | Task orchestration and remote caching for JavaScript. Its value needs multiple frontend applications, and there is one |
| Nx for JavaScript only | Splits task invocation across two tools, which driver 1 forbids. Considered above as the repo-wide runner instead |

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
├── frontend/                 # one frontend app, route groups inside (ADR-0400)
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
| Every service | `build`, `test`, `lint`, `generate`, `migrate`, `server`, `worker` — the closed set every service exposes |
| Repo root | fleet-wide work, grouped by the axis it belongs to: `ci:*` for what CI invokes, `cluster:*` for the local environment, `gen:*` for codegen, `lint:*` and `format:*` for the checks. `mise tasks --list` is the enumeration |

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

### One frontend app

The frontend is one application with route groups for `(landing)`, `(panel)`, and `(devportal)`. The framework it is built with is [ADR-0400](0400-frontend.md); the count is this ADR's.

| Reason | Detail |
| --- | --- |
| Cross-subdomain auth is hostile | Sharing cookies and session across subdomains is a recurring source of subtle production bugs, Safari ITP especially |
| Bundle size is not the constraint | Route-level code splitting weakens the separate-apps argument |
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

That generated code is committed and drift-checked is [ADR-0000](0000-platform-foundations.md), principle 9. This ADR fixes where it lands and how the check runs: `libs/{go,ts}/sdks/<service>/` for OpenAPI clients, and `services/<service>/internal/store/` for sqlc output.

`mise run ci:gen` regenerates everything and CI fails on a diff. A lefthook pre-commit hook runs the relevant slice when source files change.

### Caching

| Cache | Keyed on |
| --- | --- |
| Go build and test | the root `go.sum` |
| Docker BuildKit layers | the registry |
| Bun install | `bun.lock` |

**Deferred:** a remote Go build cache (`GOCACHEPROG`, `sccache`). **Trigger:** CI time consistently exceeding 15 minutes on the median affected pull request. **Seam:** it is a `GOCACHEPROG` environment variable, so adoption is configuration. **Cost if adopted late:** the CI minutes already spent, and nothing structural — the cache is cold on adoption whenever that happens.

### Container images

One multi-stage `Dockerfile` per service: stage 1 builds with the workspace `go build`, stage 2 is the runtime base below. One Dockerfile for the frontend, using standalone output. There is no shared base image.

| Runtime base | Verdict |
| --- | --- |
| **`gcr.io/distroless/static-debian13`** | **Chosen.** No shell and no package manager, so a compromised process has nothing to escalate with, and the CA bundle, `/etc/passwd`, and timezone data a service making outbound TLS calls needs are maintained upstream |
| `scratch` | Smaller and genuinely empty, which means hand-copying the CA bundle, a nonroot passwd entry, and tzdata, and owning the CA refresh when roots rotate |
| Alpine | A shell and a package manager in production, and musl resolves DNS differently from glibc under Go |
| Chainguard / Wolfi | Comparable hardening with a stronger SBOM story. Its freely available tags track `latest`, which the floating-tag rule forbids |

**This base is unrelated to the host operating system.** It contributes no kernel and no init — a few megabytes of files whose provenance happens to be a Debian release. Changing what the nodes run changes nothing here ([ADR-0200](0200-cluster-topology.md)).

**Server and worker are separate images from the same Dockerfile.** A `CMD` build arg selects which `cmd/` binary the build stage compiles, producing `<service>-server:<sha>` and `<service>-worker:<sha>`. They scale independently, have different resource profiles, and should not carry each other's code surface; one Dockerfile keeps the build stage and base layers shared and cache-friendly.

Images are tagged `<service>:<git-sha>`, and the same SHA flows through every environment ([ADR-0201](0201-gitops.md)).

### Upgrade path

Each step is its own ADR when triggered.

| Step | Trigger | Seam | Cost if adopted late |
| --- | --- | --- | --- |
| Split the Go module | a service or library needs its own dependency line — external publication, divergent upgrade cadence, isolated security review | present: packages already sit at import paths that become module paths unchanged | grows with the number of cross-package imports that have to become versioned dependencies, so the later it happens the more of the repository it touches |
| Adopt Nx or moon as a task orchestrator with caching and a project graph | the task graph outgrows mise, or hand-written affected detection stops being trustworthy | present: tasks are declarative and already named `group:member`, and both wrap them rather than replacing them | low, and flat — the wrapping is mechanical whenever it happens |
| Adopt Pants, Bazel, or Nix | hermetic reproducible builds become a compliance requirement. Which of the three is the ADR that gets written then, not now | **none. This is a bet** ([ADR-0000](0000-platform-foundations.md)): each replaces the build rather than wrapping it, so every Dockerfile, task, and codegen step is rewritten | flat — the rewrite is the same size whenever it happens, which is what makes deferring it free |

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

## Rules

- The fleet lives in one repository. Moving any part of it to a second repository requires its own ADR. `(review-only)`
- The repo is a single Go module rooted at `go.mod`. There are no per-service or per-library `go.mod` files and no `go.work`. `(CI: ci-lint)`
- Every backend service lives at `services/<name>/`; every shared Go package under `libs/go/<name>/`; every shared TypeScript library under `libs/ts/<name>/`. `(CI: lint:service-contract)`
- Generated API clients live at `libs/{go,ts}/sdks/<service>/` and are committed. `(CI: ci-drift)`
- The frontend is one application at `apps/frontend/`. A new frontend or a new entry under `apps/` requires an ADR. `(review-only)`
- Tasks are invoked through `mise run <task>`. Every service exposes `build`, `test`, `lint`, `generate`, `migrate`, `server`, `worker`. `(CI: lint:service-contract)`
- A task name is `group:member`, grouped by the axis worth listing together. `(review-only)`
- Every external tool is pinned: developer and CI tools in `.mise.toml`, runtime services as an explicit `image.tag` in Helm values. Floating tags are not used anywhere. `(CI: lint:floating-tags)`
- A PR changing a spec, SQL query, or any codegen input includes the regenerated artifacts. `(CI: ci-drift)`
- A change to `go.mod`, `go.sum`, root `package.json`, `infra/`, or `tools/` triggers a full-repo CI run. `(CI: ci-affected)`
- Container images are tagged `<service>:<git-sha>`, and the same SHA flows through every environment. `(CI: lint:floating-tags)`
- `services/<X>/` does not import `services/<Y>/`. Sharing happens through `libs/` or generated clients. `(CI: ci-lint)`
- Route groups inside `apps/frontend/` do not import from each other. `(CI: ci-lint)`
- Build-graph and hermetic-build tools — Nx, moon, Pants, Bazel, Nix — are not used on day one. Adoption requires its own ADR. `(review-only)`
