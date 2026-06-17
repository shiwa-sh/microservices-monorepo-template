# ADR-0013: Release, Tagging & Versioning

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0002](0002-monorepo.md), [ADR-0004](0004-gitops.md), [ADR-0008](0008-api-contracts.md)

## Context

The monorepo produces several distinct kinds of release artifact:

- **Service container images** — built per merge, deployed via GitOps.
- **Go libraries** under `libs/go/` consumed by services as in-repo packages of the single root module.
- **TS libraries** under `libs/ts/` consumed by `apps/frontend/` (and potentially external consumers).
- **Generated API clients** under `libs/{go,ts}/sdks/`.
- **Helm charts** under `infra/helm/` with their own `Chart.yaml` versions.
- **The frontend and admin apps** under `apps/`.

Per [ADR-0002](0002-monorepo.md), container images are tagged `<service>:<git-sha>` and the same SHA flows through every
environment ([ADR-0004](0004-gitops.md)). That covers deployment identity, but it does not answer:

- How do humans refer to "what's in production right now" in a CHANGELOG or release note?
- How does a Go library consumer pin a version?
- How is a breaking change to a service's OpenAPI contract ([ADR-0008](0008-api-contracts.md)) communicated to its
  clients?
- When does a version number actually change, and who is allowed to change it?

The repo already pins `cocogitto` in `mise.toml`, signalling intent to use Conventional Commits as the source of truth
for version bumps.

## Decision drivers

1. **Per-component versions, not a repo-wide version.** A monorepo with ~100 services cannot share one version number
   without becoming meaningless.
2. **Conventional Commits are mandatory.** They are the input cocogitto reads to compute bumps and changelogs.
3. **Tags, not files, are the source of truth for "released."** Version files exist where a build system needs them
   (`Chart.yaml`, `package.json`); the git tag is what `cocogitto` and humans agree on.
4. **Deploy identity stays SHA-based.** Semver tags are *additional* labels on the same image, never a replacement.
   GitOps continues to roll forward by SHA per [ADR-0004](0004-gitops.md).
5. **Releases are explicit acts.** Merging to `master` deploys; cutting a release tag is a separate command.

## Decisions

### Commit message format: Conventional Commits, enforced

All commits on `master` follow the [Conventional Commits](https://www.conventionalcommits.org/) spec:

```text
<type>(<scope>): <subject>

[body]

[footer]
```

- **Types** allowed: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `chore`, `build`, `ci`, `revert`.
- **Scope** is the component path slug: `gateway`, `clay`, `libs/go/observability`, `frontend`, `helm/postgres`, etc.
  The set of valid scopes is generated from the repo layout and enforced by lint.
- **Breaking changes** are marked with `!` after the type/scope (`feat(gateway)!: …`) **or** a `BREAKING CHANGE:`
  footer. Either form triggers a major bump.

`cocogitto` enforces the format via a lefthook `commit-msg` hook. CI re-runs `cog check` on the PR's commit range.

PR squash-merges produce a single commit whose title MUST be a valid Conventional Commit. Merge commits are disallowed.

### Versioning scheme: SemVer per released component, with prefixed tags

Only **products** — things that get deployed or shipped — carry their own SemVer line and tag namespace. Internal
libraries do not.

| Component                      | Tag format                                                         | Pre-1.0 start |
|--------------------------------|--------------------------------------------------------------------|---------------|
| Service `<S>`                  | `services/<S>/v<MAJOR>.<MINOR>.<PATCH>`                            | `v0.1.0`      |
| Helm chart `<C>`               | `helm/<C>/v<MAJOR>.<MINOR>.<PATCH>`                                | `v0.1.0`      |
| `apps/frontend`                | `apps/frontend/v<MAJOR>.<MINOR>.<PATCH>`                           | `v0.1.0`      |
| `apps/admin`                   | `apps/admin/v<MAJOR>.<MINOR>.<PATCH>`                              | `v0.1.0`      |
| Generated client `<S>` (Go/TS) | tracks the service it was generated from; not tagged independently | —             |
| Go libraries (`libs/go/*`)     | not tagged                                                         | —             |
| TS libraries (`libs/ts/*`)     | not tagged                                                         | —             |

Prefixed tags keep namespaces from colliding across many components and give every product the same uniform pattern.

Internal libraries are consumed in-tree (Go: same root module per [ADR-0002](0002-monorepo.md); TS: `workspace:*`)
and always build from on-disk source. A tag would not change what gets compiled, so we don't cut one. If a library
is ever extracted to its own repo or published externally, tagging is introduced at that point — not before.

Services remain on `0.x` until they have a stable external contract (typically when at least one external consumer
exists). Internal-only services may stay on `0.x` indefinitely; this is not a defect.

### Container image tags: SHA primary, semver label

[ADR-0002](0002-monorepo.md) and [ADR-0004](0004-gitops.md) keep `<service>:<git-sha>` as the deploy-identity tag. On a
release-tag push, the release workflow additionally pushes one immutable label:

- `<service>:v<MAJOR>.<MINOR>.<PATCH>`

GitOps manifests never reference the semver tag — it exists for human and external-documentation use only. CI greps
Helm values and GitOps manifests to enforce SHA-only references. Moving tags (`latest`, `vMAJOR.MINOR`, `vMAJOR`) are
never pushed.

### Release process: `mise run release -- <component>`

Releases are explicit per component, and a release **is** the prod deploy. Dev and staging deploy continuously from
`master` (throttled by ArgoCD sync windows — see [ADR-0004](0004-gitops.md)); prod only moves when someone cuts a tag.

A maintainer runs:

```text
mise run release -- services/gateway
```

The task:

1. Runs `cog bump --auto` scoped to the component's path, computing the next version from commits since the previous
   tag for that component.
2. Updates the component's version file if one exists (`Chart.yaml`, `package.json`).
3. Regenerates the component's `CHANGELOG.md` via `cog changelog`.
4. Commits with `chore(<scope>): release v<X.Y.Z>` and creates the prefixed tag.
5. Pushes the commit and tag.

The release commit goes through the standard build path on `master`, producing `<service>:<sha>`. The tag push then
triggers the prod promotion workflow described in [ADR-0004](0004-gitops.md)'s Image promotion section, which also
attaches the semver label and creates the GitHub Release from the cocogitto changelog.

### Changelog: per released component, generated, committed

Each tagged component (services, apps, Helm charts) owns a `CHANGELOG.md` at its root, regenerated by `cog changelog`
on every release. Internal libraries have no changelog — their history lives in `git log libs/<lang>/<name>/`, filtered
by Conventional Commit type when needed. The file
is committed because:

- It is the human-readable artifact attached to the GitHub Release.
- Reviewers can see what the release will say *in the release PR*, not after the fact.
- It survives the loss of git history (mirrors, shallow clones, etc.).

Cocogitto's default Keep-a-Changelog format is used unmodified.

### Pre-release and hotfix

- **Pre-release tags** (`-rc.<N>`, `-alpha.<N>`, etc.) are valid SemVer pre-release identifiers but **do not trigger
  prod deploys**. They exist for the rare case where an external consumer needs a named pre-release artifact (e.g., a
  partner SDK). Day-one platform work has no such consumer; this path is reserved, not the default.
- **Hotfixes** branch from the release tag's commit, not from `master`: `hotfix/<component>/v<X.Y.Z+1>`, cherry-pick the
  fix, run `mise run release` from the hotfix branch. The hotfix tag drives the same prod-promotion workflow. Hotfix
  branches are deleted after the tag exists.

### Pinning consumers

- Go consumers resolve `libs/go/*` as packages of the single root module ([ADR-0002](0002-monorepo.md)) — always the
  on-disk source. There is no external Go consumer, so the module proxy and per-library version pinning are not in
  play.
- TS consumers resolve `libs/ts/*` via `workspace:*` per [ADR-0002](0002-monorepo.md). Always the on-disk source.
- Generated API clients are not versioned independently; consumers within the repo use the workspace version.
- If a library is ever extracted to its own repo or published externally, introduce tagging and pinning at that point.

### Repo-wide version: none

There is intentionally no top-level repo version, no `v1.0.0` for "the platform." Anything that wants to say "the
platform as of date X" refers to the git SHA.

## Consequences

### Positive

- Each released component evolves at its own pace; a `fix(gateway)` doesn't drag every other service's version forward.
- Conventional Commits + cocogitto give deterministic, reviewable version bumps with no human picking numbers.
- Deploy identity (SHA) and release identity (semver) are decoupled, preserving the GitOps invariants from
  [ADR-0004](0004-gitops.md).
- Skipping library tags removes ceremony that wouldn't change any build output, since workspace mode always resolves
  libs from on-disk source.
- Prefixed tag namespace scales to hundreds of products without collisions.

### Negative / Risks

- Tag volume still grows with service count. Mitigated by per-component prefixes (easy to filter) and by routine `git gc`.
- Cocogitto's monorepo support is competent but not as mature as `release-please` or `changesets`. Re-evaluated if the
  tool becomes a bottleneck.
- Scope validation requires keeping the allowed-scope list in sync with the repo layout. Generated, not hand-maintained.
- Hotfix-from-tag adds branch ceremony but is unavoidable in a "deploy from master" world where master may already have
  unreleased work.
- Cross-team coordination on breaking library changes relies on commit hygiene and PR review rather than a version bump.
  If this becomes painful at scale, tagging can be opted in per-library without revisiting this ADR's structure.

### Follow-ups

- `cog.toml` at repo root: configured scopes, hooks, monorepo package map.
- `mise run release` task wiring (delegates to `scripts/release.sh`).
- Lefthook `commit-msg` hook running `cog verify`.
- `.github/workflows/promote-on-release.yml`: on tag push, label image with semver, bump prod values, create GitHub
  Release.
- Lint rule rejecting non-SHA image tags in Helm values and GitOps manifests.
- Tag-prefix → component-path mapping documented in `docs/release.md` (operator runbook).

## Rules

- Every commit on `master` is a valid Conventional Commit; CI fails otherwise.
- Services, apps, and Helm charts carry their own SemVer line and prefixed tag namespace. Internal libraries are not
  tagged; they are consumed via workspace mode and always built from on-disk source.
- Container images are deployed by `<service>:<git-sha>`. A single immutable `<service>:v<X.Y.Z>` label is published on
  release-tag push for human reference. Moving tags (`latest`, `vMAJOR.MINOR`, `vMAJOR`) are never published. GitOps
  manifests reference SHA tags only.
- Releases are cut by `mise run release -- <component>`; there is no auto-release on merge to `master`.
- A release tag triggers the prod deploy. Dev and staging deploy continuously from `master`, throttled by ArgoCD sync
  windows per [ADR-0004](0004-gitops.md).
- Each tagged component (services, apps, Helm charts) owns a committed `CHANGELOG.md` regenerated by cocogitto.
  Internal libraries have no changelog.
- Hotfixes branch from the release tag, not from `master`.
- There is no repo-wide version.
