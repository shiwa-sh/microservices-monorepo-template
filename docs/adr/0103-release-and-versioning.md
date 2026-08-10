# ADR-0103: Release, Tagging & Versioning

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0101](0101-monorepo.md), [ADR-0201](0201-gitops.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0500](0500-observability.md)

## Context

The monorepo produces service container images, Go and TypeScript libraries, generated API clients, Helm charts, and two applications. [ADR-0101](0101-monorepo.md) tags images `<service>:<git-sha>` and the same SHA flows through every environment. That covers deployment identity and leaves three questions open:

- How do humans refer to what is in production, in a changelog or a release note?
- When does a version number change, and who changes it?
- How is the running build's identity confirmed, rather than assumed?

**The decisive fact is that this repo ships as a single unit.** GitOps rolls one SHA through every environment, and production moves as a whole when a release is cut. No component has an external consumer that pins it independently: Go libraries are in-tree packages of one module, TypeScript libraries resolve via `workspace:*`, and service-to-service calls ride the in-repo generated SDK at HEAD.

A per-component SemVer line would therefore encode a compatibility promise **no external consumer is there to read**.

## Decision drivers

1. **The unit of versioning is the unit of shipping.** GitOps rolls one SHA through every environment and production moves as a whole, so a version line finer than that describes a boundary the system does not have.
2. **A version signals something to someone, or it signals nothing.** Whatever the number promises, some consumer has to be in a position to read it and act on it.
3. **Commit history is an input, not only a record.** The changelog and the breaking-change signal are derived from it, so its format has to be machine-readable and enforced at write time rather than tidied afterwards.
4. **Deploy identity stays SHA-based** ([ADR-0101](0101-monorepo.md)). Inherited, not decided here: any release label is additional to the image identity, never a replacement for it.
5. **The promotion boundary is a choice, not a default.** Dev and staging deploy continuously from `master` ([ADR-0201](0201-gitops.md)); whether production follows automatically is this ADR's to decide.
6. **The running build's identity is provable**, not inferred from the pipeline having succeeded.

## Considered options

### The version line

| Option | What the number signals | Human judgement per release | Verdict |
| --- | --- | --- | --- |
| **[CalVer](https://calver.org/) `vYYYY.0M.MICRO`, one repo-wide line** | when it shipped | none — it is the date | **Chosen.** It answers the only question asked of a release version |
| [SemVer](https://semver.org/), one repo-wide line | a compatibility contract | bump level, every release | The contract has no independent pinner, so the signal is addressed to nobody and the judgement call is pure cost |
| SemVer per component, prefixed tag namespaces | per-component compatibility | one judgement per component | Multiplies the same unread promise by the component count, plus tag bookkeeping |
| Git SHA only, no human version | nothing | none | Deploy identity is already the SHA. A changelog and a release note need a label a human can say aloud |

Driver 2 is what decides this row: SemVer's number is addressed to a pinner, and there is none.

**Escape hatch.** If a library, SDK, or service is extracted and published for external consumers to pin, *that artifact* adopts SemVer with its own prefixed tag, because an external pinner does need the breaking signal.

| Field | Value |
| --- | --- |
| **Trigger** | an artifact is published for a consumer outside this repository to pin |
| **Seam** | present: the repo-wide line is unprefixed, so a namespaced tag coexists with it rather than replacing it, and the chosen tooling supports per-package tagging as configuration |
| **Cost if adopted late** | paid by the first external consumer, who pins a CalVer tag that promises them nothing. The retrofit cannot start at `v1.0.0` honestly, because versions they already depend on exist |

### Commit convention

| Option | Machine-readable | How a breaking change is marked | Verdict |
| --- | --- | --- | --- |
| **Conventional Commits** | **a specified grammar with parsers in every ecosystem** | `!` after the type or scope, or a `BREAKING CHANGE:` footer | **Chosen.** Driver 3: the changelog and the breaking signal are derived rather than curated |
| Free-form messages, changelog written by hand | no | by whoever remembers | Makes the changelog a release-time authoring task and the breaking signal a matter of memory |
| gitmoji | a prefix set, with no grammar behind it | no defined marker | Expressive and popular, and there is nothing for a gate to key off |
| A house prefix scheme | as far as we build it | as far as we build it | The same grammar, authored and maintained here, with no third-party parser to inherit |

### Release tooling

| Option | Computes the version | Enforces the grammar | Verdict |
| --- | --- | --- | --- |
| **cocogitto, changelog-only** | not used — CalVer comes from the date | **yes, at `commit-msg` and in CI** | **Chosen.** One binary covers enforcement and changelog rendering, and the version it could compute is not wanted |
| commitlint | no | yes | The same enforcement as a Node program, which [ADR-0100](0100-language-and-runtime.md) bars for authored tooling |
| release-please | yes, inferring SemVer bumps from commit history | no | Its entire value is bump inference, which the version-line decision above makes moot |
| changesets | yes, per package, from author-written intent files | no | Built for independently published packages, which the same decision rules out |

## Decision

### Conventional Commits, enforced

Every commit on `master` follows the [Conventional Commits](https://www.conventionalcommits.org/) spec.

| Element | Value |
| --- | --- |
| Types | `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `chore`, `build`, `ci`, `revert` |
| Scope | the component path slug — `gateway`, `frontend`, `libs/go/observability`, `helm/postgres`. The valid set is generated from the repo layout and lint-enforced |
| Breaking change | `!` after the type or scope, or a `BREAKING CHANGE:` footer |

Under CalVer a breaking marker computes no version bump. It headlines the changelog, flags the change for review, and for the API surface it is the signal [ADR-0303](0303-api-contracts-and-lifecycle.md) keys off.

`cocogitto` enforces the format through a lefthook `commit-msg` hook, and CI re-runs `cog check` over the pull request's commit range as part of `mise run ci:lint` ([ADR-0102](0102-source-control-and-ci.md) keeps CI logic in tasks rather than workflow steps). A squash-merge produces one commit whose title is a valid Conventional Commit. Merge commits are not used.

### One repo-wide CalVer line

The repository has one release version: **`vYYYY.0M.MICRO`** — `v2026.07.0`, then `v2026.07.1` for a second release the same month, `v2026.08.0` in August. It is the human-readable alias for the state of the whole repo at that release.

Services, apps, and charts carry no version of their own. Where a build tool requires a version field, it is stamped to the current CalVer at release rather than maintained independently.

**One calendar spans release and API contract.** [ADR-0303](0303-api-contracts-and-lifecycle.md) keeps the API single-live-version by default and, only when an external consumer exists, mints date-based API versions from this same calendar. The API version is the subsequence of release dates on which the public contract changed.

### Image tags: SHA and digest for machines, CalVer for humans

| Tag | Purpose | Where it is referenced |
| --- | --- | --- |
| `<service>:<git-sha>` | deploy identity | GitOps manifests in dev and staging |
| `repo@sha256:…` digest | the strongest identity — a digest cannot be re-pushed | GitOps manifests in prod, via `image.digest`, which takes precedence over `image.tag` |
| `<service>:v<YYYY.0M.MICRO>` | human and external-documentation reference | never referenced by GitOps |

The CalVer label is pushed once per image on a release-tag push and is immutable. Moving tags are never published, and CI rejects them.

### Proving which build is running

Two distinct problems sit behind "is the pod running the new code": **actual drift**, meaning old code under a new tag, and the **observability gap**, meaning no way to confirm without trusting the pipeline.

**Drift is prevented structurally.** Deploys use immutable SHA tags, GitOps references SHA or digest only, and moving tags are never published, so a node cannot silently serve a stale image under a reused tag. Two hardenings belong to the promotion workflow ([ADR-0201](0201-gitops.md)): pin production by digest, and **gate on rollout completion** — Argo reporting Healthy is not the same as rolled out, so the workflow waits for the new ReplicaSet to become available.

**One build identity is compiled into the artifact**, never read from a runtime environment variable or ConfigMap. The point is to prove which build is running, and a runtime value can change independently of the code it describes.

| Artifact | Mechanism |
| --- | --- |
| Go services | `-ldflags -X` injects git SHA, release version, and build time into `libs/go/buildinfo`, falling back to the Go VCS stamp for a local build |
| Frontend | the same values inlined into `NEXT_PUBLIC_SERVICE_VERSION` at build |

Build scripts and CI pass these as `--build-arg GIT_SHA=… BUILD_VERSION=… BUILD_TIME=…`.

That one value surfaces three ways:

| Surface | Form | Answers |
| --- | --- | --- |
| Telemetry attribute | `obs.Init` sets OTel `service.version` and `service.build.sha` ([ADR-0500](0500-observability.md)) | "what is serving production right now", as a Grafana query with no endpoint involved |
| Endpoint | `GET /version` on the admin port, beside `/livez` and `/readyz`, returning `{version, sha, builtAt}` | per-pod scriptable checks — CI smoke, an operator's `curl` |
| Response header | `X-App-Version` and `X-App-Revision` from the shared `httpmw`; the frontend stamps `X-App-Version` too | inspect any response in devtools. Comparing the browser's bundle version against the backend header also catches a **stale cached bundle**, which the backend alone cannot reveal |

### The release process

A release is repo-wide and **is** the production deploy. Dev and staging deploy continuously from `master`, throttled by Argo sync windows ([ADR-0201](0201-gitops.md)); production moves only when someone cuts a release.

`mise run release`:

1. Computes the next CalVer from the date and the last release tag — same month bumps `MICRO`, a new month resets it.
2. Stamps version fields where a build tool needs one.
3. Regenerates `CHANGELOG.md` via `cog changelog` for the commit range since the last release.
4. Commits as `chore(release): v<YYYY.0M.MICRO>` and creates the tag.
5. Pushes the commit and the tag.

The release commit goes through the standard build path on `master`, producing `<service>:<sha>`. The tag push then triggers the production promotion workflow, which attaches the CalVer image labels and creates the release from the generated changelog.

`cocogitto` is **changelog-only**: it does not compute the version, and it does enforce commit format and render notes grouped by type with breaking changes at the top.

### Changelog, pre-release, hotfix

| Concern | Rule |
| --- | --- |
| Changelog | one top-level `CHANGELOG.md`, regenerated on every release and **committed**, so reviewers see what the release will say in the release PR and it survives a shallow clone or a mirror |
| Pre-release tags | `-rc.<N>` is valid and does **not** trigger a production deploy. Reserved for when an external consumer needs a named pre-release artifact |
| Hotfix | branch from the release tag's commit rather than `master` as `hotfix/v<YYYY.0M.MICRO+1>`, cherry-pick, and run `mise run release` from the branch. The tag drives the same promotion workflow; the branch is deleted after the tag exists |

### How consumers resolve

| Consumer | Resolution |
| --- | --- |
| Go code consuming `libs/go/*` | packages of the single root module — always on-disk source |
| TypeScript consuming `libs/ts/*` | `workspace:*` — always on-disk source |
| Generated API clients | the workspace version; not independently versioned |
| An external pinner | does not exist. Introduced only through the SemVer escape hatch at publication time |

## Consequences

### Positive

- One version to reason about: what is in production is one date, the human alias of the deployed SHA.
- No human picks bump levels and no per-component tag bookkeeping exists.
- Deploy identity and release identity stay decoupled, preserving the GitOps invariants.
- One calendar spans release and API-contract versions, so there is no second versioning vocabulary.
- The running build self-reports through telemetry, an endpoint, and a header, all derived from one compiled-in value.

### Negative / Risks

- **CalVer carries no compatibility signal.** Accepted: nothing internal is externally pinned, the API's compatibility story lives in [ADR-0303](0303-api-contracts-and-lifecycle.md), and the escape hatch covers later publication.
- **An unrelated change rides the same release number.** Accepted — the repo ships as a unit, so "everything as of this date" is what the number should mean.
- **`cocogitto` is used for a fraction of its capability.** Accepted; bump mode is opted into per artifact if external publishing ever grows.

## Rules

- Every commit on `master` is a valid Conventional Commit. Breaking markers headline the changelog and flag review; under CalVer they compute no version bump. `(CI: ci:lint; ref: Conventional Commits)`
- The repository has one release version, CalVer `vYYYY.0M.MICRO`. There are no per-component version lines or prefixed tag namespaces. `(ref: CalVer)`
- Beyond the build tag ([ADR-0101](0101-monorepo.md)), production pins by digest and exactly one immutable CalVer label is published per release for human reference. Moving tags are never published. `(CI: lint:floating-tags)`
- Releases are cut by `mise run release`. There is no auto-release on merge. A release tag triggers the production deploy; dev and staging deploy continuously from `master`.
- The repo owns one committed top-level `CHANGELOG.md`, regenerated by cocogitto in changelog-only mode.
- Hotfixes branch from the release tag, not from `master`.
- Anything extracted or published for external consumers to pin adopts SemVer with its own tag at that point. `(ref: SemVer)`
- The running build's identity is compiled into the artifact and never read from a runtime environment, and is surfaced as OTel `service.version` and `service.build.sha`, a `GET /version` admin endpoint, and `X-App-Version` and `X-App-Revision` response headers.
- The promotion workflow gates on rollout completion, not on Argo reporting Healthy.
