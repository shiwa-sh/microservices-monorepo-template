# ADR-0105: Image Registry

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0102](0102-source-control-and-ci.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0104](0104-supply-chain-security.md), [ADR-0200](0200-cluster-topology.md)
- **Decides:** Images and their referrers live in zot, one instance per environment, backed by object storage.

## Context

Every deployable is a container image referenced by digest ([ADR-0103](0103-release-and-versioning.md)). [ADR-0104](0104-supply-chain-security.md) attaches a cosign signature, an SPDX SBOM, and SLSA provenance to each one, and Kyverno verifies them at admission. Those attachments are **OCI artifacts stored alongside the image**, so the registry is part of the supply-chain control, not a bucket the pipeline pushes to.

At axis B maximal the registry is a first-class decision. It also sits on the critical path of every node: a registry outage during a scale-up or a node replacement stalls pod starts for any image not already cached.

## Decision drivers

1. **OCI 1.1 referrers**, so signatures, SBOMs, and attestations live with the image ([ADR-0104](0104-supply-chain-security.md)).
2. **Thinnest viable platform** ([ADR-0000](0000-platform-foundations.md), principle 2). The registry is one concern, and must not arrive with a datastore fleet.
3. **A pull depends on as little as possible.** Every node start reads the registry, so whatever it is coupled to becomes a dependency of scheduling itself.
4. **Configuration in the repository** (principle 1). Projects, quotas, and retention are files, not UI state.

Object storage as the backend ([ADR-0207](0207-cluster-storage.md)) is a platform constraint rather than a driver: every option below supports it, so it selects nothing and appears in the Decision instead.

## Considered options

| Option | Always-on workloads | OCI 1.1 artifacts | Config as files | Verdict |
| --- | --- | --- | --- | --- |
| **zot** | **one Go binary** | [native](https://zotregistry.dev/latest/), with S3-compatible storage | a single committed config file | **Chosen.** The only option that closes the concern without expanding the floor *(reasoned)* |
| Harbor | registry, registry controller, core, jobservice, portal, **its own Postgres and a Redis** | yes | partly — projects, robots, and retention are API and UI objects, reconciled only by a separate operator | Feature-complete and the reflexive answer. Five workloads and two datastores for one concern is the purchase principle 2 exists to refuse |
| [Project Quay](https://github.com/quay/quay) | registry, **its own Postgres and a Redis**, and Clair beside it for scanning | yes | a config bundle, authored through its own config tool | Harbor's shape under a different name. The scanning it brings is the gate [ADR-0104](0104-supply-chain-security.md) already runs in CI |
| CNCF Distribution | one binary | yes — it is the reference implementation, and the [referrers API](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers) arrived in its 3.x line, so the long-deployed 2.x images answer only through the fallback tag schema | one config file | The thinnest of all. No authentication model beyond htpasswd or a token service, no retention policy, no UI — each of which then becomes its own decision |
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
| Storage backend | the object storage in [ADR-0207](0207-cluster-storage.md). Image data is never on a node volume |
| Artefacts | OCI 1.1 referrers hold the cosign signature, SBOM, and provenance from [ADR-0104](0104-supply-chain-security.md) |
| Authentication | pipeline credentials push; cluster credentials pull. Both are SOPS-encrypted ([ADR-0202](0202-secrets.md)) |
| Third-party images | pulled from upstream and pinned by digest ([ADR-0104](0104-supply-chain-security.md)). The registry does not proxy them by default |
| Retention | untagged manifests unreferenced by a signature or an environment's values are garbage-collected on a schedule |

### Where the pipeline pushes

The registry the pipeline pushes to is **configuration, not a constant**. A project generated from this template has a forge and no cluster, so the default is the forge's own registry; a deployment that runs zot re-points the pipeline by setting two forge variables and two secrets, and no workflow is edited.

| Setting | Default | Set it to |
| --- | --- | --- |
| `IMAGE_REGISTRY` | `ghcr.io` | the registry's origin ([ADR-0306](0306-trust-tiers-and-urls.md)) |
| `IMAGE_REPOSITORY` | the forge repository path | the path images live under |
| `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` | the forge's own token | the push identity above |

This is the same property the thin-YAML rule buys for build logic ([ADR-0102](0102-source-control-and-ci.md)): moving is a re-target rather than a rewrite. A registry that could only ever be the forge's would make the pipeline the thing that has to change when the platform grows its own.

**Scanning is not the registry's job.** [ADR-0104](0104-supply-chain-security.md) makes it a merge gate in CI. zot can run a scanner; it stays off, so the concern stays in one place ([ADR-0000](0000-platform-foundations.md), principle 5).

### Mirroring upstream images is deferred

| Field | Value |
| --- | --- |
| **Trigger** | an upstream registry rate-limits or removes a pinned digest the cluster depends on |
| **Seam** | ✓ zot supports pull-through mirroring in the same config file. Enabling it changes registry configuration and image references, not the deploy path |
| **Cost if adopted late** | a bootstrap depends on upstream availability, which [`docs/guide/http-proxy.md`](../guide/http-proxy.md) already documents as a failure mode on restricted networks |

## Mirroring the upstream registries

zot is also a **pull-through cache** for the registries the platform pulls from —
docker.io, quay.io, ghcr.io and registry.k8s.io — fetching an image on first
request and keeping it.

The reason is not registry performance, it is **egress surface**. Every layer that
pulls an image otherwise needs its own network configuration: each Talos node, each
kind node, each build host. On a proxied or firewalled network that is the same
credential and the same allow-list entry maintained in four places, and the failure
when one is missed does not mention the network — a Talos node reports
`403 Forbidden` fetching etcd and the cluster never bootstraps, or Traefik sits in
`ContainerCreating` while quay.io times out mid-handshake. Both were measured
during this platform's own bring-up.

Pointing every node at zot collapses that to one component with egress and one
firewall rule.

| Option | Verdict |
| --- | --- |
| **zot's `sync` extension, `onDemand`** | **Chosen.** zot is already the registry, so this is configuration rather than a new component, and it handles all four upstreams in one instance. Nothing has to be preloaded and no image list has to be derived or kept current *(reasoned)* |
| A push-filled mirror | Works, and the platform keeps it as `mise run cluster:preload` for the local tiers. It needs a list of images derived from the charts, and that list is the part that rots — an image nobody thought to add falls back to the upstream and the mirror bought nothing |
| `registry:2` as a pull-through cache | The obvious tool and it cannot do this: it proxies exactly **one** upstream per instance, so four upstreams is four registries to run |
| Per-node proxy configuration, no mirror | Correct, and it is the same configuration repeated in four places — every one of which has to be right, and none of which says so when it is not |

**Fallback to the upstream registry stays enabled.** An image the mirror does not
have still resolves, slowly, rather than failing — the mirror is an accelerator,
not an allow-list. The allow-list is Kyverno's
([ADR-0104](0104-supply-chain-security.md)), and that separation is deliberate: a
cache that silently became an authorization boundary would be one nobody could
safely flush.

**Docker Hub is `onDemand` only and must never be polled.** It rate-limits pulls
and does not support catalog listing, so a scheduled sync walks a catalog that is
not there and spends the rate limit discovering that.

**What this does not do.** It covers image pulls. The BUILD path still needs egress
for base images and language modules, and non-image traffic — ACME, mail delivery,
DMARC reports — is untouched. This reduces the number of layers that need a proxy;
it does not remove the proxy.

## Consequences

### Positive

- One binary on the floor instead of five workloads and two datastores.
- Registry durability is object-storage durability, and a registry rebuild is a redeploy rather than a restore.
- Signatures and SBOMs live with their images, so [ADR-0104](0104-supply-chain-security.md)'s admission check is a registry read.

### Negative / Risks

- **The registry is on the critical path for pod starts.** A registry outage stalls scale-ups and node replacements for uncached images. Mitigated by object-storage-backed statelessness, which makes recovery a redeploy.
- **zot is a younger project than Harbor**, with a smaller operator population. Accepted under principle 4 on exit cost: images are re-pushable and the OCI API is the interface. **The exit cost is not the whole cost.** The operator population is a second, independent price — nobody arriving has debugged this component before, so the first incident is also the first hour anyone has spent inside it. That is the same class of cost [ADR-0200](0200-cluster-topology.md) accepts openly for Talos, and it is paid at the worst moment rather than at adoption.
- **No web UI worth the name.** Inspecting an image is a `crane` or `cosign` call. Accepted: image inspection is an engineer's task, not an operator's dashboard ([ADR-0501](0501-operator-uis-and-dashboards.md)).
- **Robot credentials are long-lived** where the forge cannot mint short-lived ones, which is the same constraint [ADR-0102](0102-source-control-and-ci.md) records for signing identity.

### What would change this decision

| Change | Effect |
| --- | --- |
| The forge is replaced | **None.** No forge's bundled registry is eligible, whichever forge it is, for the coupling reason above |
| Multi-tenancy, quotas, or replication becomes a requirement | **Decisive.** Those are the capabilities Harbor was rejected for carrying, and needing one of them means the concern grew past what a single-instance registry answers |
| The registry needs an administrative console for a non-engineer | **Decisive**, and it is the accepted absence stated above rather than a discovered gap |
| Scanning is wanted at the registry rather than in CI | **None.** [ADR-0203](0203-policy-enforcement.md) assigns provenance to admission and scanning to the merge gate; moving it here would be a policy-layer change, not a registry choice |
| The image estate outgrows one instance per environment | **None** on the component, decisive on its topology: zot mirrors in the same config file, which is the recorded seam |

## Rules

- Images are stored in a self-hosted zot registry backed by object storage.
- Registry configuration is a committed file. Projects, quotas, and retention are never set through an API call or a UI.
- Signatures, SBOMs, and provenance are OCI referrers on the image they describe ([ADR-0104](0104-supply-chain-security.md)). `(ref: OCI 1.1)`
- Vulnerability scanning runs in CI, not in the registry.
- The registry the pipeline pushes to is set by forge variables, never written into a workflow.
- Deployments reference images by digest ([ADR-0103](0103-release-and-versioning.md)). `(enforced: Kyverno)`
