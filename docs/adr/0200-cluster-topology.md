# ADR-0200: Cluster Topology & Hosting

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0003](0003-naming-and-identifiers.md), [ADR-0201](0201-gitops.md), [ADR-0205](0205-environment-parity.md), [ADR-0302](0302-temporal.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0500](0500-observability.md), [ADR-0501](0501-operator-uis-and-dashboards.md)

## Context

Three environments — dev, staging, prod — are one cluster each. Workloads span stateless application services, stateful platform components, and ingress.

This ADR answers where production runs, what shape a cluster has on day one and how it grows, how traffic reaches a pod, and how storage and backups work. What may differ between environments is [ADR-0205](0205-environment-parity.md); the laptop is [ADR-0600](0600-local-development-loop.md).

## Decision drivers

1. **Operational sovereignty** ([ADR-0000](0000-platform-foundations.md), principle 3). Nobody outside the organisation can change the terms on which the control plane runs, and per-cluster, per-load-balancer and per-volume fees do not compound across a growing fleet.
2. **A node matches its description.** The gap between what a host is declared to be and what it has become is a failure class in its own right, and one this layer can remove rather than monitor.
3. **The pod network is not a trust boundary by default.** Whatever provides the CNI decides whether a compromised pod can reach Postgres, and that is settled at bootstrap.
4. **Boring by default, novel where it removes a class of failure** ([ADR-0000](0000-platform-foundations.md), *spend novelty by exit cost*). A less widely operated component is adopted where it eliminates a category of failure rather than improving on one, and the ADR names the category.
5. **Losing one node is not downtime.**

Manifest-layer parity is a constraint inherited from [ADR-0205](0205-environment-parity.md) rather than a driver here: topology may differ between environments, charts and commands may not.

## Considered options

### Node operating system

| Option | Shell and SSH on the node | Configuration | Kubernetes | Verdict |
| --- | --- | --- | --- | --- |
| **Talos Linux** | **neither — an API is the only interface** | a declarative machine config document applied over gRPC | upstream, shipped and upgraded with the OS | **Chosen** |
| Flatcar Container Linux | retained, and some two thousand binaries with them | Ignition at provision time, and whatever mutates the host afterwards | installed separately | The CoreOS lineage, and it keeps the shell and the drift a shell permits. Only `/usr` is read-only |
| Fedora CoreOS | retained | Ignition and rpm-ostree | installed separately | As Flatcar, and it does not assume Kubernetes, so the parts this platform never uses are still attack surface |
| Bottlerocket | disabled by default, reachable through an admin container | API-driven, closest in spirit to Talos | installed separately | The nearest philosophical match. Its platform support is AWS-centric and bare metal is poorly documented, which driver 1 makes decisive |
| Debian stable, converged by Ansible | full | playbooks converging a mutable host | k3s, installed by playbook | The honest baseline. A node matches its description only to the degree the playbooks are complete, and re-running them is the only evidence |

**The node is the only mutable thing left in the system.** Cluster state is reconciled from git ([ADR-0201](0201-gitops.md)), pods run on a base with no shell and no package manager ([ADR-0101](0101-monorepo.md)), and the pod network is default-deny. Against that posture the host shell is the one remaining escalation path and the one remaining source of drift, and an immutable node collapses the description and the thing described into a single object. That is the failure class driver 4 requires naming.

Driver 4 also admits the cost: this is a less widely operated OS than Debian, and the ADR pays for it in *Consequences* rather than pretending otherwise.

### East-west security, which is a CNI-level choice

Linkerd is not a CNI and not a Cilium substitute — it rides on top of a CNI through per-pod sidecars. The day-one decision is therefore at the CNI layer, compared on security capability.

| Capability | flannel only | Calico | flannel + Linkerd | **Cilium + WireGuard** |
| --- | --- | --- | --- | --- |
| L3/L4 default-deny segmentation | **none — flat network** | all pods | meshed app traffic only | all pods |
| Data-tier protection (Postgres, MinIO, OpenFGA) | wide open | NetworkPolicy | only if the data tier is meshed, which is fiddly | NetworkPolicy |
| Cryptographic workload identity | — | — | mTLS certs, meshed only | label identity; SPIFFE optional later |
| Encryption in transit, east-west | **plaintext** | WireGuard, and it does not also replace kube-proxy | meshed only | all pods, WireGuard |
| L7 authz by route and method | — | limited, and Envoy-based in the paid tier | fine-grained | coarse, via Envoy — already covered by Oathkeeper and OpenFGA |
| Egress control, DNS/FQDN, metadata SSRF | — | FQDN egress in the paid tier | not Linkerd's concern | FQDN and L3 egress in the open distribution |
| Per-flow visibility | — | a separate collector | mesh-only | Hubble, from the same datapath |

**flannel ships no NetworkPolicy controller**, so the API server accepts a policy and nothing enforces it — worse than having no policy, because it grants false confidence. Calico is the closest real alternative and loses on scope rather than on capability: its FQDN egress and L7 controls sit in the paid tier, and it leaves kube-proxy in place, which the Talos configuration below removes. Between the remaining options, Cilium wins on **breadth**: the controls it adds map onto the highest-frequency cluster attacks — lateral movement to Postgres, and metadata-endpoint credential theft. The controls Linkerd adds over Cilium are depth *behind* those, and the L7 layer is already covered at the edge and in-app. Linkerd would additionally leak the data tier unless the stateful components are meshed, which the Job-heavy bootstrap makes painful.

A sidecar mesh also adds one proxy container per pod on the hot path, against [ADR-0000](0000-platform-foundations.md)'s per-service cost principle.

### Kubernetes distribution

| Option | Installer to operate | Coupling to the node OS | What it bundles | Verdict |
| --- | --- | --- | --- | --- |
| **Upstream Kubernetes, shipped by Talos** | none | one artefact, one upgrade path | nothing beyond Kubernetes | **Chosen.** The distribution and the OS upgrade together, and there is no installer standing between them |
| k3s | its own | independent | Traefik, ServiceLB, `local-path`, CoreDNS, all replaceable | Those bundles are the real loss here, and each is replaceable by a chart this repository already commits. Its lighter footprint belongs to the single-node SQLite configuration, which three-node HA forecloses by requiring etcd. **Resources decide this row in neither direction**, and it is conformant, so it is not a smaller Kubernetes to graduate from later |
| k0s | its own | independent | nothing | k3s without the bundles, which removes the only real distinction from the chosen option while keeping the separate installer |
| Full upstream kubeadm | its own | independent | nothing | The same upstream Kubernetes with an installer to operate, which is the part Talos removes |
| Managed Kubernetes (EKS, GKE, AKS) | none — the provider's | none | the provider's opinions | Fails driver 1: the terms on which the control plane runs are the provider's, and per-cluster fees compound across environments |

### Day-one node count

| Option | Failure behaviour | Migration to HA later | Verdict |
| --- | --- | --- | --- |
| **Three nodes, embedded etcd** | tolerates single-node loss with no downtime | none needed | **Chosen.** Embedded-etcd quorum needs three, and starting there removes the rebuild entirely |
| One node | multi-minute downtime on any node failure | a rebuild, with a data migration | Fails driver 5 at every scale |
| Two nodes | **worse than one** — no quorum, and two failure domains to lose it in | a rebuild | Even quorum is the classic mistake this row exists to name |
| Three nodes, etcd on dedicated hosts | as chosen | none | The same guarantee for more machines. It becomes right when etcd contends with workloads, which is a growth trigger rather than a day-one shape |

### Block storage

Object storage carries the durable data ([ADR-0205](0205-environment-parity.md)), so this decides only the volumes that back a pod between reschedules.

| Option | Added components | Survives node loss | Talos cost | Verdict |
| --- | --- | --- | --- | --- |
| **`local-path-provisioner`** | one small provisioner | **no** — a volume is pinned to its node | none: a directory under `/var`, the writable path | **Chosen.** The durable data is already off-cluster, so replication here would be paid twice |
| Longhorn | a controller, per-node engines, and a UI | yes, replicated | `iscsi-tools` and `util-linux-tools` extensions, a data path under `/var/mnt`, and a separate disk | The answer once a volume is too large to lose. It is the storage trigger below, and it is an OS-image change rather than a chart |
| OpenEBS Mayastor | a control plane and per-node data planes | yes | hugepages and a dedicated device | Longhorn's guarantee at higher performance and higher operational surface, for a workload profile no component here has |
| Rook-Ceph | a full Ceph cluster | yes, strongly | extensions plus dedicated disks | A distributed storage system to operate, which principle 2 refuses for volumes that hold no durable data |
| A provider CSI driver | none in-cluster | yes | none | Fails driver 1, and re-couples the cluster to one provider's API |

## Decision

### Hosting

Production runs on **plain compute instances**, never a provider's managed Kubernetes. How the instances come to exist is per project; two modes are supported against the same downstream bootstrap.

| Mode | Provisioning | Bucket |
| --- | --- | --- |
| Project provisions its own infrastructure | Terraform under `infra/terraform/` creates instances, network, LB, DNS, firewall, and bucket, isolating the provider behind a stable interface. Swapping providers is a module swap, not a topology change | created |
| Infrastructure is pre-provided | Terraform is skipped. The machine configs are applied to existing Talos nodes named in a committed inventory | referenced by configuration |

**The dividing line is provisioning only.** Everything downstream — machine configuration, Kubernetes, Cilium, Argo CD — is identical. Terraform belongs to the project that owns its infrastructure, and the second mode never invokes it.

**Pre-provided means pre-provided Talos.** Talos is installed by booting its own image, not converged onto a running general-purpose distribution, so this mode requires nodes already running Talos and reachable on its API. A pre-provided fleet running anything else is a reprovision, not a configuration step.

The cost of self-hosting is operational, and the machine configuration under `infra/talos/` is that operational knowledge in code — a document per node role rather than a procedure that converges a host.

### Node OS

**Talos Linux on every node.** There is no SSH, no shell, no package manager, and no writable root filesystem. Configuration is a machine config document applied over the node's gRPC API, and `talosctl` is the only other interface.

| Concern | Mechanism |
| --- | --- |
| Configuration | a machine config document per node role, committed, applied by the [`siderolabs/talos`](https://registry.terraform.io/providers/siderolabs/talos/latest) Terraform provider |
| OS upgrade | `talosctl upgrade` — an A/B image swap with rollback, one node at a time |
| Kubernetes upgrade | `talosctl upgrade-k8s`, versioned independently of the OS |
| Anything outside the base image — drivers, iSCSI, GPU | a system extension baked into a custom installer image through [Image Factory](https://docs.siderolabs.com/talos/latest/learn-more/image-factory), referenced by schematic and pinned like any other artefact ([ADR-0104](0104-supply-chain-security.md)) |
| Machine secrets | the cluster CA and `talosconfig`, SOPS-encrypted in git like every other secret ([ADR-0202](0202-secrets.md)) |

Nothing runs on these nodes outside Kubernetes. A host agent, a debugging shell, and a one-off manual fix are unavailable by construction, which is the property being bought rather than a limitation being tolerated.

### Topology and growth

Day one, per environment: three compute nodes running Talos, with etcd on all three. All workloads run on this set, sized for many cores and generous NVMe.

Each growth step is a deferral, and carries all three fields.

| Trigger | Response | Seam | Cost if adopted late |
| --- | --- | --- | --- |
| Sustained CPU or memory above 70% for 7 days across the node set | add worker agents; the control plane stays at three | ✓ a worker's machine config is a committed document and joining is an apply. No workload is pinned to a node | capacity is added under pressure rather than ahead of it, so the window is an incident rather than a change |
| Any PVC exceeding 50% of node disk | Longhorn becomes the default storage class | ⚠ **a bet.** Longhorn needs system extensions, a `/var/mnt` data path, and a separate disk, so adopting it rebuilds the installer image and reprovisions every node. There is no slot waiting for it | every existing volume migrates per workload while the schematic changes underneath, which is two migrations at once rather than one |
| Regulated data carrying an isolation requirement | a dedicated cluster for that workload | ✓ every environment is already one cluster built from the same committed configuration, so another is a values selection ([ADR-0205](0205-environment-parity.md)) | the regulated data has already been co-resident, which no later separation undoes |

### Traffic flow

```text
Internet
  │
Provider Load Balancer  (L4, one stable public IP per env)
  │
Traefik  (TLS via cert-manager, L7 routing, rate limiting)
  ├── <host>/api/*                        ─▶ Oathkeeper ─▶ backend service   (ADR-0305)
  ├── <host>/(landing|panel|devportal)/*  ─▶ frontend pod                     (ADR-0400)
  ├── lowdefy.ops.<host>/                 ─▶ Oathkeeper ─▶ Lowdefy            (ADR-0401)
  ├── grafana.ops.<host>/                 ─▶ Oathkeeper ─▶ Grafana            (ADR-0501)
  └── hubble.ops.<host>/                  ─▶ Oathkeeper ─▶ Hubble UI          (ADR-0501)
```

**Traefik is the only ingress. Oathkeeper is an auth filter behind it, not a second gateway.** Traefik does TLS, routing, load balancing, and rate limiting; Oathkeeper validates identity and injects identity headers ([ADR-0305](0305-edge-auth-and-traffic-policy.md)). There is no API-management gateway in the default stack.

**DNS.** One wildcard `A` record per environment points at the LB IP, and cert-manager requests one wildcard certificate per environment via DNS-01. `external-dns` is not used, because the wildcard absorbs new services.

### Cluster networking

Cilium is the CNI from day one. The [machine config](https://docs.siderolabs.com/kubernetes-guides/cni/deploying-cilium) sets `cluster.network.cni.name: none` and `cluster.proxy.disabled: true`, so Talos ships neither its default CNI nor kube-proxy, and Cilium provides both. Cilium is delivered as an inline manifest in the machine config rather than installed afterwards: a node reports `NotReady` until a CNI runs, and a cluster left without one reboots to retry, so shipping it with the bootstrap removes a timed race. Argo CD adopts the release afterwards for upgrades. **CNI cannot be hot-swapped on a live cluster**, so the security posture is set at bootstrap rather than retrofitted.

Three Talos properties constrain how Cilium is configured, and each is an invariant rather than a preference:

| Constraint | Consequence |
| --- | --- |
| Workloads may not load kernel modules | `SYS_MODULE` is dropped from Cilium's default capability set |
| kube-proxy is absent | Cilium is given the API server host and port directly, because there is no in-cluster Service through which to discover it |
| **[KubeSpan](https://docs.siderolabs.com/talos/latest/networking/kubespan) is not enabled** | Talos's own WireGuard mesh intercepts inter-node traffic that Cilium's eBPF datapath expects on the primary interface, producing asymmetric routing and broken cross-node pod traffic. East-west encryption is Cilium's, once |

Three postures are on from day one and are checked invariants:

| Posture | Mechanism |
| --- | --- |
| **Default-deny segmentation** | The `platform-baseline` CiliumNetworkPolicy sets `enableDefaultDeny` for ingress and egress across every platform pod. All allows are additive, and each service's chart declares which callers may reach it |
| **Encryption in transit** | `encryption.type: wireguard` encrypts all east-west pod traffic node to node. A config flag, not a component |
| **Egress control** | Default-deny egress means no pod reaches the internet unless a policy grants it. A clusterwide policy additionally denies `169.254.169.254/32`, so even a future broad egress grant cannot become an SSRF path to instance credentials |

Default-deny is load-bearing rather than defence in depth: internal calls carry forwarded identity headers and no token ([ADR-0305](0305-edge-auth-and-traffic-policy.md)), so **NetworkPolicy is what guarantees only sanctioned callers reach a service's port**.

Hubble provides per-flow visibility and is the audit surface for these policies, through the CLI, the drop metrics in Grafana, and the auth-gated UI ([ADR-0501](0501-operator-uis-and-dashboards.md)). Application observability is Grafana's ([ADR-0500](0500-observability.md)).

Cilium covers CNI and mesh as one component: sidecarless eBPF gives transparent encryption, L7 policy, and per-flow observability without an injected proxy. Cilium mutual auth and SPIFFE can add certificate identity later without sidecars.

### Storage

| Kind | Day one | On the storage-scale trigger |
| --- | --- | --- |
| Block | `local-path-provisioner`, backed by a directory under `/var`, which is the writable path on an otherwise read-only node | Longhorn becomes the default for new volumes |
| Object, production | an external S3-compatible bucket per environment. **No MinIO in production** | unchanged |
| Object, non-prod | in-cluster MinIO exposing the same S3 API ([ADR-0205](0205-environment-parity.md)) | unchanged |

**The storage trigger is an image change, not a chart change** — [Longhorn on Talos](https://longhorn.io/docs/latest/advanced-resources/os-distro-specific/talos-linux-support/) requires work at the OS layer, which is why the deferral above is labelled a bet rather than a seam.

Loki, Tempo, CNPG backups, and Pyroscope write to the bucket. Prometheus keeps a local TSDB and needs none; its Mimir Scale swap does ([ADR-0500](0500-observability.md)). Offloading durability to an external bucket eliminates a stateful component from production.

### Backups, mandatory and off-cluster

| Data | Mechanism | Retention |
| --- | --- | --- |
| Postgres | CNPG `ScheduledBackup` to the external bucket, with WAL archiving for PITR | 30 days prod, 7 days non-prod |
| Temporal history | lives on Postgres, so it is covered above | as above |
| Observability long-term | already in the bucket; the cluster volume holds hot cache only | per lifecycle policy |
| Whole node | provider snapshots daily, as a catastrophic-recovery fallback | provider default |

Restore is rehearsed quarterly by a Temporal `Schedule` ([ADR-0302](0302-temporal.md)) that opens a tracking issue.

### Provisioning order

```text
0. terraform apply   # instances, network, LB, DNS, firewall, bucket
                     # only when the project provisions its own infra
1. terraform apply   # machine configs applied and the cluster bootstrapped
                     # through the siderolabs/talos provider; Cilium rides
                     # along as an inline manifest
2. kubectl apply -f infra/gitops/bootstrap/root-application.yaml
                     # Argo CD reconciles the rest
```

There is no configuration-management step between provisioning and Argo CD, because there is no mutable host state to converge.

Cluster identity is reproducible from git plus the SOPS-encrypted machine secrets, with one Terraform state file when the project owns its infrastructure, or the committed inventory and the referenced bucket when it does not.

### Disaster recovery

Three-node HA tolerates single-node failure with no downtime; etcd quorum survives. A full-cluster loss recovers through `terraform apply` where applicable, then machine-config apply and bootstrap, then Argo CD reconciling from git, then CNPG restoring from PITR. On pre-provided infrastructure the Talos nodes already exist, so recovery starts at the machine-config apply.

| Target | Value |
| --- | --- |
| Detection | under 2 minutes |
| Recovery | under 30 minutes |
| RPO | the WAL archive interval |

Rehearsed quarterly alongside the backup restore drill.

## Consequences

### Positive

- Three-node HA from day one removes the rebuild-to-HA migration entirely.
- **Node configuration drift is not mitigated, it is impossible.** The machine config is the node, and there is no interface through which the two can diverge.
- **The node has no shell for a compromised process to reach**, which is the posture the pods already had, now extended to the host under them.
- An OS upgrade is an image swap with a rollback rather than a package transaction with a partial-failure mode.
- The same Kubernetes API end to end: environments differ in detail, not in shape.
- Growth triggers are tied to measurable conditions.
- An external bucket removes MinIO as a production component.
- Provisioning is reproducible from git, and Terraform is not a day-one dependency.

### Negative / Risks

- **Three nodes cost more than one.** Accepted: the alternative is a maintenance window nobody wants to plan.
- **Self-hosted Kubernetes on plain compute is more operational work than managed Kubernetes.** Mitigated by the machine configuration being the codified knowledge.
- **There is no shell.** Debugging is `talosctl` — logs, the dashboard, and the API — and an engineer whose instinct is `ssh` plus `journalctl` has to relearn the loop. This is the cost of the property, not a defect.
- **Anything outside the base image is a build artefact.** A driver or kernel module becomes an Image Factory schematic and a custom installer image, pinned and tracked under [ADR-0104](0104-supply-chain-security.md) rather than installed with a package manager.
- **Nothing can run on these nodes outside Kubernetes.** A workload that needs to sit beside the cluster is foreclosed, and would need its own host.
- **k3s's bundled defaults are gone.** `local-path` is now an installed component rather than a shipped one, and a deployment without a provider load balancer needs one chosen rather than inherited from ServiceLB.
- **Talos is less widely operated than Debian**, so there is less community answer-surface when something is strange, and the failure modes are less familiar. Accepted against the drift and escalation classes it removes.
- **Cilium is harder to debug than flannel** — eBPF programs, `cilium status`, the Hubble CLI. Mitigated by the chart being committed and Argo CD managing upgrades after bootstrap.
- **Bucket fees grow with retention.** Mitigated by lifecycle policies moving to a cold tier after 30 days.
- **CNI is set at bootstrap and cannot be changed live.** This is why the security posture is decided here rather than deferred.

## Rules

- Production runs on plain compute instances, never managed Kubernetes. Terraform is a per-project tool, skipped when infrastructure is pre-provided.
- Every node runs Talos Linux, configured only by its machine config. There is no SSH, no configuration-management agent, and no manual change to a node.
- Every environment runs three control-plane nodes with etcd on each. Adding workers follows the resource-pressure trigger.
- Anything not in the base Talos image arrives as a system extension in a pinned installer image built through Image Factory.
- Ingress is Traefik with TLS from cert-manager. Oathkeeper sits behind it as the edge identity filter; there is no API-management gateway.
- Object storage in production is an external S3-compatible bucket. Non-prod uses in-cluster MinIO behind the same API.
- Database backups are written off-cluster to that bucket and the restore is rehearsed quarterly.
- The storage class is `local-path-provisioner` over a directory under `/var` until the storage-scale trigger fires, then Longhorn with the extensions that requires.
- CNI is Cilium from day one, delivered as an inline manifest in the machine config and adopted by Argo CD for upgrades. Talos ships neither its default CNI nor kube-proxy.
- KubeSpan is not enabled. East-west encryption is Cilium's WireGuard, and enabling both breaks cross-node pod traffic.
- Default-deny is enforced for ingress and egress across every platform pod; all allows are additive. `(enforced: CiliumNetworkPolicy)`
- WireGuard transparent encryption is on for all east-west pod traffic. Plaintext east-west is not shipped.
- A clusterwide policy denies `169.254.169.254/32`, so no egress grant can become a metadata-SSRF path. `(enforced: CiliumNetworkPolicy)`
- A new cluster bootstraps with the machine-config apply then the Argo CD root Application. There is no configuration-management step between them and no further manual steps.
- Growth beyond the day-one topology happens only on one of the documented triggers firing.
- No dedicated service mesh is deployed. Sidecar meshes are ruled out by per-service resource cost and component count.
- Cilium NetworkPolicy is the internal service-to-service trust boundary, and each service declares its allowed callers. `(CI: lint:service-contract)`
