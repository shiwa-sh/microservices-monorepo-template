# ADR-0105: Image Registry

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0102](0102-source-control-and-ci.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0104](0104-supply-chain-security.md), [ADR-0200](0200-cluster-topology.md)

## Context

Every deployable is a container image referenced by digest ([ADR-0103](0103-release-and-versioning.md)). [ADR-0104](0104-supply-chain-security.md) attaches a cosign signature, an SPDX SBOM, and SLSA provenance to each one, and Kyverno verifies them at admission. Those attachments are **OCI artifacts stored alongside the image**, so the registry is part of the supply-chain control, not a bucket the pipeline pushes to.

At axis B maximal the registry is a first-class decision. It also sits on the critical path of every node: a registry outage during a scale-up or a node replacement stalls pod starts for any image not already cached.

## Decision drivers

1. **OCI 1.1 referrers**, so signatures, SBOMs, and attestations live with the image ([ADR-0104](0104-supply-chain-security.md)).
2. **Thinnest viable platform** ([ADR-0000](0000-platform-foundations.md), principle 2). The registry is one concern, and must not arrive with a datastore fleet.
3. **A pull depends on as little as possible.** Every node start reads the registry, so whatever it is coupled to becomes a dependency of scheduling itself.
4. **Configuration in the repository** (principle 1). Projects, quotas, and retention are files, not UI state.

Object storage as the backend ([ADR-0200](0200-cluster-topology.md)) is a platform constraint rather than a driver: every option below supports it, so it selects nothing and appears in the Decision instead.

## Considered options

| Option | Always-on workloads | OCI 1.1 artifacts | Config as files | Verdict |
| --- | --- | --- | --- | --- |
| **zot** | **one Go binary** | native, with S3-compatible storage | a single committed config file | **Chosen.** The only option that closes the concern without expanding the floor |
| Harbor | registry, registry controller, core, jobservice, portal, **its own Postgres and a Redis** | yes | partly — projects, robots, and retention are API and UI objects, reconciled only by a separate operator | Feature-complete and the reflexive answer. Five workloads and two datastores for one concern is the purchase principle 2 exists to refuse |
| CNCF Distribution | one binary | yes — it is the reference implementation, and the referrers API arrived in its 3.x line, so the long-deployed 2.x images answer only through the fallback tag schema | one config file | The thinnest of all. No authentication model beyond htpasswd or a token service, no retention policy, no UI — each of which then becomes its own decision |
| Forgejo package registry | none — the forge is already a decided component ([ADR-0102](0102-source-control-and-ci.md)) | partial | forge config | Free in component count and rejected anyway, on the coupling argument below rather than on capability |
| Managed registry | none | yes | provider API | Fails principle 3. Ranked late on the [ADR-0000](0000-platform-foundations.md) swap list, so it is a concession taken well after the ones above it |
| Do nothing | none | n/a | n/a | The honest baseline: images live wherever CI last pushed them. Incompatible with digest-pinned admission |

Harbor loses on component weight rather than on capability. Its replication, quota, and multi-tenancy features answer a problem a single platform team with one registry does not have, and its Postgres and Redis are exactly the "always-on floor is the budget" cost principle 2 is written to stop. Its Trivy scanner is optional and would stay off regardless, for the reason below.

**No forge's bundled registry is eligible, whichever forge is chosen.** GitLab and Forgejo both ship one, so a bundled registry reads as free in component count. It is not: an artefact store inside the build system couples every pod start to forge availability, and makes the system that produces an image the system that stores what vouches for it ([ADR-0104](0104-supply-chain-security.md)).

Separating them is what lets [ADR-0102](0102-source-control-and-ci.md) hold that a forge outage does not stop the running system, and it is why the registry is a decision the forge cannot settle.

## Decision

| Concern | Decision |
| --- | --- |
| Registry | **zot**, one instance per environment, configured by a committed file |
| Storage backend | the object storage in [ADR-0200](0200-cluster-topology.md). Image data is never on a node volume |
| Artefacts | OCI 1.1 referrers hold the cosign signature, SBOM, and provenance from [ADR-0104](0104-supply-chain-security.md) |
| Authentication | pipeline credentials push; cluster credentials pull. Both are SOPS-encrypted ([ADR-0202](0202-secrets.md)) |
| Third-party images | pulled from upstream and pinned by digest ([ADR-0104](0104-supply-chain-security.md)). The registry does not proxy them by default |
| Retention | untagged manifests unreferenced by a signature or an environment's values are garbage-collected on a schedule |

**Scanning is not the registry's job.** [ADR-0104](0104-supply-chain-security.md) makes it a merge gate in CI. zot can run a scanner; it stays off, so the concern stays in one place ([ADR-0000](0000-platform-foundations.md), principle 5).

### Mirroring upstream images is deferred

| Field | Value |
| --- | --- |
| **Trigger** | an upstream registry rate-limits or removes a pinned digest the cluster depends on |
| **Seam** | ✓ zot supports pull-through mirroring in the same config file. Enabling it changes registry configuration and image references, not the deploy path |
| **Cost if adopted late** | a bootstrap depends on upstream availability, which [`docs/ops/http-proxy.md`](../ops/http-proxy.md) already documents as a failure mode on restricted networks |

## Consequences

### Positive

- One binary on the floor instead of five workloads and two datastores.
- Registry durability is object-storage durability, and a registry rebuild is a redeploy rather than a restore.
- Signatures and SBOMs live with their images, so [ADR-0104](0104-supply-chain-security.md)'s admission check is a registry read.

### Negative / Risks

- **The registry is on the critical path for pod starts.** A registry outage stalls scale-ups and node replacements for uncached images. Mitigated by object-storage-backed statelessness, which makes recovery a redeploy.
- **zot is a younger project than Harbor**, with a smaller operator population. Accepted under principle 4: a registry's exit cost is low, because images are re-pushable and the OCI API is the interface.
- **No web UI worth the name.** Inspecting an image is a `crane` or `cosign` call. Accepted: image inspection is an engineer's task, not an operator's dashboard ([ADR-0501](0501-operator-uis-and-dashboards.md)).
- **Robot credentials are long-lived** where the forge cannot mint short-lived ones, which is the same constraint [ADR-0102](0102-source-control-and-ci.md) records for signing identity.

## Rules

- Images are stored in a self-hosted zot registry backed by object storage.
- Registry configuration is a committed file. Projects, quotas, and retention are never set through an API call or a UI.
- Signatures, SBOMs, and provenance are OCI referrers on the image they describe ([ADR-0104](0104-supply-chain-security.md)). `(ref: OCI 1.1)`
- Vulnerability scanning runs in CI, not in the registry.
- Deployments reference images by digest ([ADR-0103](0103-release-and-versioning.md)). `(enforced: Kyverno)`
