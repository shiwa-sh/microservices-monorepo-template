# ADR-0102: Source Control & CI Platform

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0101](0101-monorepo.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0104](0104-supply-chain-security.md)
- **Decides:** Forgejo is the forge and Forgejo Actions the pipeline runner, entered through `mise run ci:*` so the forge stays cheap to leave.

## Context

[ADR-0000](0000-platform-foundations.md) principle 3 makes source control and CI a first-class decision rather than a signup. A forge bundles four concerns a managed provider hides: the repository, the review record, the pipeline, and the identity that signs artefacts.

The pipeline is where artefacts are built and signed ([ADR-0104](0104-supply-chain-security.md)), so whoever runs it decides what "built by us" can be proven against.

A managed forge is the one dependency that cannot be reconciled with principle 3 by pinning or by a contract, because the trust root is the thing being outsourced.

## Decision drivers

1. **Operational sovereignty** ([ADR-0000](0000-platform-foundations.md), principle 3). Source, review, and build run on infrastructure the organisation controls.
2. **Thinnest viable platform** (principle 2). A forge is one component. A suite that arrives with its own datastore fleet is a different purchase.
3. **Exit cost stays low** (principle 4). Whatever is chosen, the pipeline definition must not become where build logic lives, because then leaving means rewriting it rather than re-pointing it.
4. **One primitive per concern** (principle 5). Source, review, and pipelines are one purchase or several, and each additional system is operational surface the platform team carries.
5. **The build environment is part of the supply chain** ([ADR-0104](0104-supply-chain-security.md)). Whoever runs the pipeline can alter what is built, whatever is signed afterwards.

## Considered options

| Option | Component weight | Pipelines | Governance | Verdict |
| --- | --- | --- | --- | --- |
| **Forgejo + Forgejo Actions** | one Go binary, plus a database on the existing CNPG cluster | built in, GitHub-Actions workflow syntax, runners are self-hosted by definition | non-profit umbrella ([Codeberg e.V.](https://forgejo.org/)), [GPLv3+](https://lwn.net/Articles/986998/) | **Chosen.** The only option that satisfies drivers 1–4 together *(reasoned)* |
| Gitea + Gitea Actions | identical | identical | company-controlled | Functionally equivalent. Provenance is *recorded evidence about exit cost* (principle 4), and the governing body is the only discriminator between two otherwise equal choices |
| GitLab CE | a suite — Gitaly, Redis, Sidekiq, its own Postgres, several web services | mature and complete | company-controlled, open-core | Rejected by principle 2, as [ADR-0000](0000-platform-foundations.md) already records. It replaces one component with a platform |
| Self-hosted forge + separate CI engine | two components | Woodpecker, Tekton, or Argo Workflows | varies | Rejected by principle 5, as [ADR-0000](0000-platform-foundations.md) already records: a second system beside the forge |
| A managed forge — the do-nothing baseline | none | hosted runners | third-party | Fails driver 1 outright. It leaves axis B asserted rather than held, and the identity that signs artefacts in a third party's control |

Forgejo and Gitea are the same software lineage and score identically on every technical row. Principle 4 says provenance does not veto a choice but is recorded; here it is the only distinguishing evidence, so it decides.

### The two tools the pipeline runs on

Both Tier 2 ([ADR-0002](0002-tool-adoption.md)), and both are consequences of the runner shape rather than independent preferences.

| Concern | Chosen | Picked over | Why |
| --- | --- | --- | --- |
| Image builds | **BuildKit, rootless** | Buildah, Kaniko, a socket-mounted Docker daemon | The node is immutable and exposes no Docker socket ([ADR-0200](0200-cluster-topology.md)), so a socket-mounted builder is unavailable rather than rejected *(reasoned)*. Of the three that remain, BuildKit is the one the `Dockerfile` syntax is defined against, so cache mounts and multi-stage behaviour need no translation. Buildah is the runner-up and its exit is a task change |
| Running a workflow locally | **act** | pushing to a branch, running a self-hosted runner on the workstation | Workflow YAML is a thin caller of `mise run ci:*`, so most local verification is running the task directly. `act` covers the remaining case — the workflow wiring itself — without a push |

## Decision

| Concern | Decision |
| --- | --- |
| Forge | **Forgejo**, self-hosted, backed by a database on the existing CNPG cluster ([ADR-0300](0300-data.md)) rather than its own |
| Pipelines | **Forgejo Actions**, with runners on infrastructure we control |
| Workflow content | checkout, toolchain setup, then `mise run ci:*`. Logic does not live in YAML |
| Review record | pull requests on the forge. Branch protection and required checks are configuration in the repository ([ADR-0000](0000-platform-foundations.md), principle 1) |
| Registry | **not** the forge's package registry. [ADR-0105](0105-image-registry.md) decides it separately |

**Workflow YAML is a portable subset.** Steps are `actions/checkout`, the repository's own composite setup action, and `mise run` calls. That subset runs on both Forgejo Actions and the provider-hosted workflows the template ships, which is what makes the forge cheap to leave in either direction.

### The template ships provider-hosted workflows

A generated project needs CI from its first commit, before any cluster exists to run a forge on. The workflows under `.github/workflows/` are therefore the bootstrap path, not a competing decision.

| Field | Value |
| --- | --- |
| **Trigger** | the platform cluster serves its first environment |
| **Seam** | ✓ the thin-YAML rule above. Moving is a re-targeting of the same `mise run ci:*` calls, not a rewrite of pipeline logic |
| **Cost if adopted late** | the pipeline migration is flat — the thin-YAML rule makes it a re-target rather than a rewrite. **The review record is not flat.** Pull requests, review comments, and issues are forge-native and do not travel with a `git push`, so that body grows for as long as the decision waits. Sovereignty is also asserted rather than held until it lands |

### Signing identity

[ADR-0104](0104-supply-chain-security.md) signs with a key pair held in SOPS rather than against a third-party certificate authority, precisely so the signing identity does not depend on which forge runs the pipeline. Moving the forge re-targets the workflow and leaves the trust root untouched.

Forgejo does mint [per-job OIDC tokens for Actions](https://forgejo.org/docs/latest/user/actions/security-openid-connect/), alongside ephemeral runners. That capability is available for anything wanting a short-lived workload identity; it is not what signs images.

## Consequences

### Positive

- Source, review, pipelines, and the build identity run on controlled infrastructure. Axis B is held rather than asserted.
- Forgejo Actions reuses the workflow syntax already in the repository, so the migration is re-targeting rather than rewriting.
- No second CI engine, and no second datastore: the forge uses the Postgres already on the floor.

### Negative / Risks

- **The forge joins Core**, and a forge outage blocks merges and builds. Argo CD keeps reconciling from the last synced commit, so a forge outage does not stop the running system.
- **Self-hosted runners are compute the platform team owns**, including their cache and their isolation. Untrusted-PR execution is a policy question a public repository must answer before enabling it.
- **Forgejo's pipeline feature set trails the provider it imitates.** The thin-YAML rule bounds the exposure: the surface used is checkout, setup, and a task call.

## Rules

- The forge is Forgejo, self-hosted, with its database on the existing CNPG cluster.
- Pipelines run on Forgejo Actions with runners on controlled infrastructure. No second CI engine is introduced ([ADR-0000](0000-platform-foundations.md), principle 5).
- Workflow YAML checks out, sets up the toolchain, and calls `mise run ci:*`. Pipeline logic is not written in YAML.
- Branch protection and required checks are configuration in the repository, never set through the forge UI ([ADR-0000](0000-platform-foundations.md), principle 1).
- Container images are published to the registry in [ADR-0105](0105-image-registry.md), not to the forge's package registry.
