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
| Authentication | pipeline credentials push; cluster credentials pull. Both are SOPS-encrypted ([ADR-0202](0202-secrets.md)). The pull credential reaches workloads as an `imagePullSecret`, not as node configuration — containerd 2 ignores the node's registry auth once a hosts.d config path is set, which Talos always sets ([infra/talos](../../infra/talos/README.md)) |
| Third-party images | pinned by digest ([ADR-0104](0104-supply-chain-security.md)) and served through the registry's `sync` extension, which mirrors every upstream the platform pulls from |
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

### Mirroring the upstream registries

zot is also a **pull-through cache** for every upstream the platform pulls from, and on the local tiers it is the **only** source: each upstream is mirrored at the one address, none is configured as a fallback endpoint, and `cluster:up` fills the cache before it creates the cluster.

The reason is egress surface rather than registry performance. Every layer that pulls an image otherwise carries its own network configuration — each deployed node, each local kind node, each build host — so on a proxied network one credential and one allow-list entry are maintained once per puller, and the failure when one is missed never names the network: a node reports `403 Forbidden` fetching etcd and never bootstraps, or Traefik sits in `ContainerCreating` while an upstream times out mid-handshake. Pointing every node at zot collapses that to one component with egress and one firewall rule.

| Option | Verdict |
| --- | --- |
| **zot's `sync` extension in `onDemand` mode, filled ahead of the cluster** | **Chosen.** zot is the registry already, so this is configuration rather than a component, and one instance serves every upstream *(reasoned)* |
| `onDemand` alone, filled by the cluster's own pulls | zot copies a whole image before it answers the manifest request that triggered it, so a cold pull costs tens of seconds. A bring-up asks for its images at once: zot saturates, every pull exceeds containerd's deadline, and the pods back off while zot goes on caching images nothing waits for. Most of a full tier's pods sit in `ImagePullBackOff` against a registry that is answering correctly — re-derived by emptying the cache directory and running `cluster:up full` *(measured)* |
| A push-filled mirror | It needs a list of images derived from the charts, and a hand-maintained list rots: an image nobody adds falls back to the upstream and the mirror bought nothing. An image behind a chart's conditional is invisible to any derivation that does not render the chart |
| `registry:2` as a pull-through cache | The obvious tool, and it proxies exactly **one** upstream per instance, so each upstream is another registry to run |
| Per-node proxy configuration, no mirror | Correct, and it is the same configuration repeated once per puller — every copy of which has to be right, and none of which says so when it is not |

**The local tiers run the same registry, as a host container.** A laptop's mirror is zot rather than a second product, so the mirroring above is the mirroring a developer gets and the image path is not a place the environments differ ([ADR-0205](0205-environment-parity.md)). It sits beside the cluster rather than inside it: the in-cluster instance stores its images in the object store, and cannot serve the images the object store's own pods need to start. Two deltas are local-only and both are properties of a throwaway host container — its storage is a directory rather than a bucket, and it serves anonymously, because a credential on a laptop mirror guards nothing and every pull would carry it.

**The in-cluster instance still runs on the full local tier — as an exercised chart, not as the node's registry.** Every platform chart is applied there so a chart change is caught before a deployed environment ([ADR-0205](0205-environment-parity.md)), and zot is no exception: it deploys, is backed by the local object store, and is reached at `zot.ops.<host>`. What it is *not* is the source the nodes pull from — that is the host container above, because `*.localtest.me` does not resolve inside a node ([ADR-0306](0306-trust-tiers-and-urls.md)), a node-provisioning delta [ADR-0205](0205-environment-parity.md) permits. So its catalogue is empty by default, which reads as a broken console rather than the parity artifact it is. `cluster:up full` therefore mirrors the first-party images into it after the platform is healthy — the same images CI pushes to the in-cluster registry in a deployed environment — so its push, object-store, and console path is exercised locally rather than only in production. The nodes keep pulling from the host container; the in-cluster instance holds the same images so nothing about it is untested but the one hop the node networking forbids.

**Why the registry is in-cluster at all** is the placement rule [ADR-0207](0207-cluster-storage.md) states: a component lives off-cluster only where its data must outlive the cluster. The registry's contents are reproducible — re-pushed by CI, or re-read from a surviving bucket — so it needs no independent survival and stays in-cluster, where the object store it depends on already is. The backup store is the opposite case and goes off-cluster in production for exactly that reason.

**The cache is filled before the cluster exists, and the local nodes have no fallback.** `cluster:up` warms zot from a generated list, sequentially, with nothing waiting on it, and the nodes then read locally. A miss fails loudly and names the image instead of becoming a slow direct pull that differs per machine.

An upstream second endpoint cannot serve as the safety net it resembles: containerd falls through to the next endpoint only on a fast failure, and a saturated mirror fails by timing out, which ends the pull. What it delivers is an image path that varies with the network, so none is configured.

**This is not an authorization boundary.** The allow-list is Kyverno's ([ADR-0104](0104-supply-chain-security.md)), and that separation is deliberate: a cache that silently became an authorization boundary would be one nobody could safely flush. Being the only *route* is a determinism property; what may run is still admission's question.

**The warm list is generated, never written by hand**, which is what disqualifies the push-filled mirror above rather than the mirroring itself. `mise run gen:image-allowlist` renders every chart and emits both Kyverno's repository allow-list and `infra/local/image-refs.txt`, the full references the warm reads. One render, two outputs, drift-checked in CI, so the warm set cannot fall behind the charts and an image behind a chart's conditional is as visible as any other.

**Docker Hub is `onDemand` only and is never polled.** It rate-limits pulls and does not support catalog listing, so a scheduled sync walks a catalog that is not there and spends the rate limit discovering it.

**Image pulls are the whole of what this covers, and not all of them.** `kind` downloads its node image through the host's docker, and the build path needs egress for base images and language modules. Non-image traffic — chart repositories, the git remote Argo syncs, ACME, mail delivery, DMARC reports — is untouched. This reduces the number of layers that need a proxy; it removes neither the proxy nor the network from a local bring-up.

## Consequences

### Positive

- One binary on the floor instead of five workloads and two datastores.
- Registry durability is object-storage durability, and a registry rebuild is a redeploy rather than a restore.
- Signatures and SBOMs live with their images, so [ADR-0104](0104-supply-chain-security.md)'s admission check is a registry read.

### Negative / Risks

- **The registry is on the critical path for pod starts.** A registry outage stalls scale-ups and node replacements for uncached images. Mitigated by object-storage-backed statelessness, which makes recovery a redeploy.
- **zot is a younger project than Harbor**, with a smaller operator population. Accepted under principle 4 on exit cost: images are re-pushable and the OCI API is the interface. **The exit cost is not the whole cost.** The operator population is a second, independent price — nobody arriving has debugged this component before, so the first incident is also the first hour anyone has spent inside it. That is the same class of cost [ADR-0200](0200-cluster-topology.md) accepts openly for Talos, and it is paid at the worst moment rather than at adoption.
- **The console reads; it does not administer.** zot's `ui` and `search` extensions serve the catalogue at `zot.ops.<host>` ([ADR-0306](0306-trust-tiers-and-urls.md)), which answers "what is in the registry" without a shell. Projects, quotas and retention remain committed files with no screen that writes one, and scripted inspection is a `crane` or `cosign` call. CVE scanning is a sub-key of the same extension and is off: it pulls a vulnerability database on a schedule, and scanning belongs to the merge gate ([ADR-0203](0203-policy-enforcement.md)).
- **The console does not ask for a second login.** zot's access policy is one configuration for one process, so a browser that cleared the ops gate would otherwise meet the same htpasswd the distribution API uses. An anonymous read policy would remove the prompt, but it opens every pull on `registry.<host>` to do it. So the credential is presented for the browser instead: a small reverse proxy rides in the zot pod, adds the pull identity's `Authorization` header, and serves the `zot.ops.<host>` origin ([ADR-0306](0306-trust-tiers-and-urls.md)). The `registry.<host>` origin still reaches zot directly and its clients authenticate as before — the header is added on the console path alone.
- **Robot credentials are long-lived** where the forge cannot mint short-lived ones, which is the same constraint [ADR-0102](0102-source-control-and-ci.md) records for signing identity.

### What would change this decision

| Change | Effect |
| --- | --- |
| The forge is replaced | **None.** No forge's bundled registry is eligible, whichever forge it is, for the coupling reason above |
| Multi-tenancy, quotas, or replication becomes a requirement | **Decisive.** Those are the capabilities Harbor was rejected for carrying, and needing one of them means the concern grew past what a single-instance registry answers |
| The registry needs an administrative console for a non-engineer | **Decisive.** The console above reads the catalogue; a surface that writes projects, quotas or retention is the capability Harbor was rejected for carrying |
| Scanning is wanted at the registry rather than in CI | **None.** [ADR-0203](0203-policy-enforcement.md) assigns provenance to admission and scanning to the merge gate; moving it here would be a policy-layer change, not a registry choice |
| The image estate outgrows one instance per environment | **None** on the component, decisive on its topology: zot mirrors in the same config file, which is the recorded seam |

## Rules

- Images are stored in a self-hosted zot registry backed by object storage.
- Registry configuration is a committed file. Projects, quotas, and retention are never set through an API call or a UI.
- The registry console is served at `zot.ops.<host>` behind the ops forward-auth; the distribution API at `registry.<host>` is gated by the registry's own credentials and never by an operator session ([ADR-0306](0306-trust-tiers-and-urls.md)).
- Every environment's registry is zot, including the local tiers, where it runs as a host container beside the cluster ([ADR-0600](0600-local-development-loop.md)). Anonymous access and directory storage are permitted there and nowhere else.
- The local nodes pull from that registry and from nowhere else: no upstream is configured as a fallback endpoint, and `cluster:up` warms the registry before it creates the cluster.
- The warm set is generated from the charts alongside Kyverno's allow-list, never hand-written. `(CI: lint:image-allowlist)`
- Signatures, SBOMs, and provenance are OCI referrers on the image they describe ([ADR-0104](0104-supply-chain-security.md)). `(ref: OCI 1.1)`
- Vulnerability scanning runs in CI, not in the registry.
- The registry the pipeline pushes to is set by forge variables, never written into a workflow.
- Deployments reference images by digest ([ADR-0103](0103-release-and-versioning.md)). `(enforced: Kyverno)`
