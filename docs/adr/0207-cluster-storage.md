# ADR-0207: Cluster Storage & Backups

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0200](0200-cluster-topology.md), [ADR-0205](0205-environment-parity.md), [ADR-0300](0300-data.md), [ADR-0302](0302-temporal.md), [ADR-0500](0500-observability.md)
- **Decides:** Volumes are node-local, object storage is SeaweedFS in every environment and outside the cluster in production, and backups are written to it under Object Lock.

## Context

[ADR-0200](0200-cluster-topology.md) puts three Talos nodes per environment on plain instances. Two storage questions follow and are separable: what backs a pod's volume between reschedules, and where durable data lives.

They are separable because the answer to the second removes most of the first. Once Postgres backups, logs, traces, profiles, and the registry's own store are in object storage, a block volume holds cache and working state, and replicating it would be paying for durability twice.

The store that holds durable data is also **what a cluster is rebuilt from**, so it cannot share a failure domain with the cluster it restores. That constraint is what makes this a decision rather than a component choice.

Parity is inherited from [ADR-0205](0205-environment-parity.md): topology may differ between environments, interfaces may not.

## Decision drivers

1. **The recovery path survives losing the cluster.** Whatever the backups are on is out of scope for the failure it recovers from.
2. **The recovery path survives a credential.** An operator credential that can delete backups is an incident away from a total loss, so immutability is a requirement rather than a feature.
3. **One implementation everywhere** ([ADR-0205](0205-environment-parity.md)). S3 dialects differ in exactly the places that fail late, and a difference only production exercises is a difference discovered in production.
4. **Operational sovereignty** ([ADR-0000](0000-platform-foundations.md), principle 3). The store is a component the platform runs, not a signup.
5. **The always-on floor is the budget** (principle 2). Durability is bought once, at the layer that needs it.

## Considered options

### Block storage

Object storage carries the durable data, so this decides only the volumes that back a pod between reschedules.

| Option | Added components | Survives node loss | Talos cost | Verdict |
| --- | --- | --- | --- | --- |
| **`local-path-provisioner`** | one small provisioner | **no** — a volume is pinned to its node | none: a directory under `/var`, the writable path | **Chosen.** The durable data is already off-cluster, so replication here would be paid twice *(reasoned)* |
| Longhorn | a controller, per-node engines, and a UI | yes, replicated | `iscsi-tools` and `util-linux-tools` extensions, a data path under `/var/mnt`, and a separate disk | The answer once a volume is too large to lose. It is the storage trigger below, and it is an OS-image change rather than a chart |
| OpenEBS Mayastor | a control plane and per-node data planes | yes | hugepages and a dedicated device | Longhorn's guarantee at higher performance and higher operational surface, for a workload profile no component here has |
| Rook-Ceph | a full Ceph cluster | yes, strongly | extensions plus dedicated disks | A distributed storage system to operate, which principle 2 refuses for volumes that hold no durable data |
| A provider CSI driver | none in-cluster | yes | none | Fails driver 4, and re-couples the cluster to one provider's API |

### Object storage

One component, in every environment. The parity contract is not "the S3 API" but **the same implementation** (driver 3), because the dialects differ in multipart, conditional writes, checksum trailers, and list pagination.

**In production it runs outside the cluster.** That is the whole environment delta, and it is driver 1. External means external to the cluster, not to the organisation: hardware the organisation owns satisfies it, and a provider bucket is the [`adoption-path.md`](../adoption-path.md) concession rather than the default.

Selection is therefore one comparison against one set of requirements: it runs at `instances=1` on a laptop ([ADR-0600](0600-local-development-loop.md)) *and* as a durable off-cluster store holding Postgres backups with their WAL archive, Loki, Tempo, and Pyroscope.

Because that store holds the backups an operator credential can reach, driver 2 makes **Object Lock a requirement rather than a feature**: it is the only control that survives an attacker holding valid credentials.

| Option | One component everywhere | Object Lock | Storage cost | Operator UI | Verdict |
| --- | --- | --- | --- | --- | --- |
| **SeaweedFS** | **yes** — `weed mini` is the whole store in one process for local and dev; master, volume, filer and S3 off-cluster in production, from the same binary | **yes** — versioning with GOVERNANCE and COMPLIANCE retention | erasure coding on warm data, ~1.4× | master, filer and admin UIs, started by the same command | **Chosen.** The only option that holds driver 3 and driver 2 together *(reasoned)* |
| Garage | yes — a single binary in both | **no**, and not reachable: Object Lock requires versioning, which it does not implement | replication only, **3×** | none first-party; a CLI and an HTTP admin API | The most elegant fit and the lightest to operate. Conceding Object Lock here is a **bet rather than a deferral** — there is no seam, and adopting it later is an object-store migration with every byte moved |
| Ceph RGW | **no** — cannot run in the local loop at any configuration | yes, the most complete | erasure coding, tunable | Ceph Dashboard, the strongest here | Eliminated by driver 3 itself. It remains the right answer for an organisation **already operating Ceph**, where the marginal cost is a pool and a gateway rather than a second distributed system |
| MinIO | yes | yes | erasure coding | stripped from the community edition in 2025 | The incumbent, and **no longer maintained**: the repository was archived in April 2026, distribution is source-only, and the maintained build is a proprietary-licensed AIStor binary. Abandonment rather than novelty risk, the failure [ADR-0502](0502-alerting-and-on-call.md) records for Grafana OnCall |
| RustFS | yes | young | — | a console | Principle 4 places object storage in the conservative class, and this slot is unambiguously the data plane |
| A provider bucket in every environment | **no** — breaks the offline local loop and puts a credential in every developer's environment | yes | n/a | the provider's | Abandons axis B for the one Core data path where it was never recorded as abandoned. Correct as a ranked concession, not as the default |
| Two implementations, one per tier — the honest baseline | no | varies | — | varies | Buys a lighter non-prod component and pays for it with a dialect gap that only production can find |

**Neither remaining candidate was free of S3-dialect defects, and the failure modes differed.** When AWS SDKs began sending CRC32 checksum trailers by default in January 2025, Garage rejected the requests outright and SeaweedFS wrote the trailer into stored object bodies. Both are fixed. A store that corrupted quietly on the path Postgres backups use is the more serious of the two, and it is recorded in *Negative / Risks* rather than treated as history.

## Decision

| Kind | Day one | On the storage-scale trigger |
| --- | --- | --- |
| Block | `local-path-provisioner`, backed by a directory under `/var`, which is the writable path on an otherwise read-only node | Longhorn becomes the default for new volumes |
| Object | **SeaweedFS in every environment**, in-cluster for local, dev and staging, and **outside the cluster in production** | unchanged |

**Outside the cluster is a failure-domain requirement, not an implementation one.** No store placed on the cluster it serves is acceptable in production whatever it is — an objection separate from, and stronger than, the component-weight one that rejects Rook-Ceph for block storage above.

**Production buckets enable Object Lock.** The store sits inside the same administrative boundary as the cluster whose backups it holds, and WORM retention is what keeps a compromised or mistaken credential from taking the recovery path with it.

**The lock window EQUALS the backup retention — not "at least".** A longer lock is not the safer choice it looks like: CNPG deletes backups past their retention, and a lock outliving that retention makes every one of those deletes fail. The bucket then grows without bound while the operator logs delete errors nobody reads, which is a slower outage than the one the lock prevents. Equality is what makes the two agree.

**Mode is per bucket, and it turns on what the bucket holds.**

| Bucket | Mode | Why |
| --- | --- | --- |
| CNPG backups | **COMPLIANCE** | This is the bucket the lock exists for. COMPLIANCE cannot be shortened or bypassed by anyone, including the root credential — which is the whole threat: an attacker holding valid credentials, or an operator holding a bad one |
| Loki, Tempo, Pyroscope | **GOVERNANCE** | Operational telemetry with short retention and no recovery role. The ability to delete on request is worth more here than immutability, and GOVERNANCE keeps that available to an authorised identity |
| The registry's bucket | **GOVERNANCE** | Images are rebuildable from source; the bucket holds no unique state ([ADR-0105](0105-image-registry.md)) |

**COMPLIANCE collides with erasure, and the collision is disclosed rather than discovered.** A subject's data inside a locked backup cannot be deleted before the window expires — that is what COMPLIANCE means. So [ADR-0301](0301-data-lifecycle-privacy.md)'s erasure obligation has a bound this ADR sets: erasure removes the subject from every live store immediately, and from backups **by expiry** of the lock window. That bound is short precisely because the window equals the backup retention rather than exceeding it, and it is stated to a subject rather than left to be found during an audit.

Loki, Tempo, CNPG backups, and Pyroscope write to the bucket. Prometheus keeps a local TSDB and needs none; its Mimir Scale swap does ([ADR-0500](0500-observability.md)).

### The storage-scale trigger is a bet, not a seam

| Field | Value |
| --- | --- |
| **Trigger** | any PVC exceeding 50% of node disk |
| **Seam** | ⚠ **none.** [Longhorn on Talos](https://longhorn.io/docs/latest/advanced-resources/os-distro-specific/talos-linux-support/) needs system extensions, a `/var/mnt` data path, and a separate disk, so adopting it rebuilds the installer image and reprovisions every node |
| **Cost if adopted late** | every existing volume migrates per workload while the schematic changes underneath, which is two migrations at once rather than one |

### Backups, mandatory and off-cluster

| Data | Mechanism | Retention |
| --- | --- | --- |
| Postgres | CNPG `ScheduledBackup` to the external bucket, with WAL archiving for PITR | 30 days prod, 7 days non-prod |
| Temporal history | lives on Postgres, so it is covered above | as above |
| Observability long-term | already in the bucket; the cluster volume holds hot cache only | per lifecycle policy |
| Whole node | provider snapshots daily, as a catastrophic-recovery fallback | provider default |

Restore is rehearsed quarterly by a Temporal `Schedule` ([ADR-0302](0302-temporal.md)) that opens a tracking issue. The recovery objectives those backups are held to are [ADR-0200](0200-cluster-topology.md)'s.

## Consequences

### Positive

- **One object store, one dialect, one set of operator habits.** A multipart or checksum difference cannot hide until production, because every environment runs the same implementation.
- The recovery path survives both losing the cluster and losing a credential, which are the two failures a backup exists for.
- Block storage costs one small provisioner, because durability is bought once at the object layer.

### Negative / Risks

- **Production object storage is a stateful component the platform operates.** Offloading it to a provider would be a saving, and that saving is spent here to keep the one Core data path from being a signup. The obligation lands on the same team that runs the cluster.
- **SeaweedFS's S3 layer is younger than its storage layer.** When AWS SDKs began sending CRC32 checksum trailers in January 2025 it wrote the trailer into stored object bodies rather than rejecting the request — silent corruption on the path Postgres backups use, since fixed. Backup verification is what catches a recurrence, which is why the restore rehearsal is not optional.
- **Development is concentrated in one maintainer.** Accepted under principle 4 on the same terms as the registry: the exit is an S3 endpoint change plus a data copy, and the interface is the API rather than the product.
- **Erasure coding is available at a fixed ratio in the open version.** Tuning it is an upstream commercial feature. Adequate here, and recorded so it is not discovered during a capacity exercise.
- **A block volume does not survive node loss.** Correct for cache and working state, and the trigger above is what says when it stops being correct.
- **Bucket fees grow with retention.** Mitigated by lifecycle policies moving to a cold tier after 30 days.

## Rules

- The storage class is `local-path-provisioner` over a directory under `/var` until the storage-scale trigger fires, then Longhorn with the extensions that requires.
- Object storage is SeaweedFS in every environment. A second S3 implementation is not introduced for any tier.
- Production runs it outside the cluster. No object store holding production data runs on the cluster it serves.
- Production buckets have Object Lock enabled, with the lock window EQUAL to the backup retention — a longer window makes CNPG's retention deletes fail and the bucket grow without bound.
- Object Lock mode is COMPLIANCE on the backup bucket and GOVERNANCE on the telemetry and registry buckets.
- Erasure reaches live stores immediately and locked backups by expiry of the lock window, and that bound is disclosed.
- Database backups are written off-cluster to that bucket and the restore is rehearsed quarterly.
- Loki, Tempo, CNPG backups, and Pyroscope write to object storage rather than to a block volume.
