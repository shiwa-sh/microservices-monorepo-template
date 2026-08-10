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

### East-west security

**Neither layer adjacent to the CNI provides east-west security.** A service mesh sits above it, riding on a CNI; Talos's own networking sits below it, on the host. The day-one decision is therefore at the CNI layer, compared on security capability. Linkerd is the mesh column, as the lightest of them: a mesh that loses these rows in its best case loses them in every case.

**Talos's networking is node-scoped.** The [host firewall](https://docs.siderolabs.com/talos/latest/networking/host-firewall) governs the node's own ports — kubelet, etcd, the API server — and never sees pod-to-pod traffic. KubeSpan encrypts links between nodes, so it segments nothing and does not touch same-node pod traffic. Both harden the machine; driver 3 is the network between pods.

| Capability | flannel only | Calico, eBPF dataplane | flannel + KubeSpan + Calico | flannel + Linkerd | **Cilium + WireGuard** |
| --- | --- | --- | --- | --- | --- |
| Components to operate | 1 | 1 | **3** | 2 | **1** |
| L3/L4 default-deny segmentation | **none — flat network** | all pods | all pods | meshed app traffic only | all pods |
| Data-tier protection (Postgres, MinIO, OpenFGA) | wide open | NetworkPolicy | NetworkPolicy | only if the data tier is meshed, which the Job-heavy bootstrap resists | NetworkPolicy |
| Cryptographic workload identity | — | — | — | mTLS certs, meshed only | label identity; SPIFFE optional later |
| Encryption in transit, east-west | **plaintext** | all pods, WireGuard | node to node only | meshed only | all pods, WireGuard |
| Services without kube-proxy | no | yes, in the eBPF dataplane | **no — policy-only mode is iptables** | no | yes |
| L7 authz by route and method | — | limited, and Envoy-based in the paid tier | — | fine-grained | coarse, via Envoy |
| Egress control, DNS/FQDN, metadata SSRF | — | CIDR egress; **FQDN in the paid tier** | CIDR egress | not Linkerd's concern | FQDN and L3 egress in the open distribution |
| Per-flow visibility | — | **denied-packet metrics; flow logs are paid** | — | mesh-only | Hubble, from the same datapath |
| Verdict | policy is accepted and never enforced | **runner-up**; both capabilities it paywalls are committed here | most components, fewest capabilities | mesh-scoped, and additive to a CNI rather than a substitute | **Chosen** |

**flannel ships no NetworkPolicy controller**, so the API server accepts a policy and nothing enforces it — worse than having no policy, because it grants false confidence.

**Calico is the runner-up.** Its open distribution matches segmentation, WireGuard encryption of all pod traffic, and kube-proxy replacement through eBPF. Both capabilities it paywalls are load-bearing here: FQDN egress, without which a policy degrades to hand-maintained CIDR lists per external dependency, and the per-flow surface [ADR-0501](0501-operator-uis-and-dashboards.md) commits as this ADR's audit trail. Its compensating iptables dataplane, which any engineer can inspect, is forfeited by the eBPF mode that drops kube-proxy.

**flannel + KubeSpan + Calico is the Talos-native path plus policy, and costs the most for the least.** Calico over flannel runs policy-only, so the dataplane is iptables and kube-proxy stays. KubeSpan requires the [discovery service](https://docs.siderolabs.com/talos/latest/configure-your-talos-cluster/system-configuration/discovery), whose hosted endpoint is a third-party runtime dependency for east-west encryption and whose self-hosted build carries a commercial licence, failing driver 1.

**Cilium wins on breadth.** The controls it adds map onto the highest-frequency cluster attacks — lateral movement to the data tier, and metadata-endpoint credential theft — and it covers CNI, Services, encryption, and observability as one component.

#### Why no service mesh

**A mesh runs over a CNI, never instead of one.** The question is therefore not Cilium or a mesh but Cilium against Cilium plus a mesh: a second identity system and a second encrypted datapath on the same nodes, against one subtracted config flag.

**Sidecars are not what decides it.** Linkerd puts a proxy container on every pod's hot path, against [ADR-0000](0000-platform-foundations.md)'s per-service cost principle. Istio's ambient mode removes that proxy — a per-node ztunnel carries L4 and mTLS, and waypoint proxies are added only where L7 policy is wanted — and the overlap above rules it out regardless.

| What a mesh adds | Who needs it | This platform |
| --- | --- | --- |
| L7 traffic management — weighted routing, mirroring, circuit breaking | percentage-based progressive delivery | no traffic-splitting delivery is committed. **This is the trigger that reopens the row**, and the one capability here Cilium lacks |
| L7 authorization imposed from outside the application | large polyglot estates, and code that cannot be changed | Oathkeeper at the edge, OpenFGA in-app ([ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0304](0304-identity-and-authorization.md)) |
| Per-request telemetry for uninstrumented workloads | applications without tracing | every service is instrumented ([ADR-0500](0500-observability.md)) |
| Per-workload certificate identity | compliance requiring an auditable CA chain | label identity, with Cilium mutual auth and SPIFFE available later without sidecars |
| A multi-cluster fabric with locality failover | one service spanning clusters | one cluster per environment |

**FQDN egress stays a CNI job under either.** Istio's `ServiceEntry` with `REGISTRY_ONLY` matches on SNI and Host, which is policy for a cooperating workload rather than enforcement against a compromised one. Driver 3 asks for control in the datapath, and that is Cilium's in every column.

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

Nothing runs on these nodes outside Kubernetes. A host agent, a debugging shell, and a one-off manual fix are unavailable by construction, which is the property being bought rather than a limitation being tolerated. This is narrower than it sounds against [12-Factor XII](https://12factor.net/admin-processes): one-off admin work still runs, in the same image and release as the service — migrations as an init container ([ADR-0300](0300-data.md)), operator tasks through the admin UIs ([ADR-0401](0401-internal-admin.md)). What is unavailable is the host shell, not the one-off process.

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

Two provider capabilities are therefore requirements rather than conveniences, and both are verified before an environment is provisioned. **A DNS provider API that cert-manager supports**, without which DNS-01 issuance has no path. **Reverse-DNS (`PTR`) delegation on the mail egress IP**, which [ADR-0307](0307-outbound-email.md) needs to match maddy's HELO name — not every provider offers it, and where it is offered it is often manual, request-only, or unavailable for load-balancer addresses. Both are cheap to confirm at provider selection and expensive to discover afterwards: a missing `PTR` surfaces as mail being rejected at first send, not as a failed deploy.

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

### Workload hardening

Talos removes the host attack surface and Cilium removes the network's. Neither constrains what a *pod* may ask the kernel for, which is the third invariant.

**Every namespace enforces the [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/) `restricted` profile**, through the in-tree Pod Security Admission controller — namespace labels, no component:

```yaml
pod-security.kubernetes.io/enforce: restricted
pod-security.kubernetes.io/enforce-version: v1.34
pod-security.kubernetes.io/warn: restricted
pod-security.kubernetes.io/audit: restricted
```

The version is pinned rather than left at `latest`, so a Kubernetes upgrade that adds a criterion is a deliberate bump in a reviewed PR rather than a batch of pods that stop scheduling during a control-plane upgrade.

| Concern | Decision |
| --- | --- |
| Enforcement mechanism | **Pod Security Admission**, in the API server. Kyverno already runs ([ADR-0104](0104-supply-chain-security.md)) and could express the same rules, but it gates *image provenance*; workload privilege is a separate concern and PSA costs no component to enforce it (principles 2 and 5) |
| Service containers | the shared chart sets `runAsNonRoot`, a non-zero `runAsUser`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `seccompProfile: RuntimeDefault`, and `readOnlyRootFilesystem: true`. A service needing a writable path declares an `emptyDir`, never a writable root |
| Components that need more | run in **their own namespace**, labelled `privileged` or `baseline`, with the reason recorded in the namespace manifest. Cilium is the day-one case |
| Exception granularity | the namespace, because that is PSA's unit. A component needing privilege never shares a namespace with one that does not |

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
- **`restricted` breaks third-party charts.** A community chart whose image runs as root or writes to its root filesystem does not schedule, and the fix is a values override, a rebuilt image, or its own `baseline` namespace. This surfaces at install rather than at runtime, which is the intended direction, but it is friction on every new platform component.
- **PSA's exception unit is the namespace, not the workload.** One component needing privilege takes its whole namespace with it, so the namespace layout is partly dictated by privilege rather than by grouping. Kyverno could express per-workload exceptions; that is the cost of not using it here.
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
- Talos's host firewall governs the node's own ports and is never treated as pod-to-pod segmentation.
- Every namespace carries the Pod Security Standards `restricted` labels with a pinned `enforce-version`. A namespace at `baseline` or `privileged` records why in its manifest. `(enforced: PSA)`
- Service containers run as non-root with a read-only root filesystem, `ALL` capabilities dropped, `allowPrivilegeEscalation: false`, and `seccompProfile: RuntimeDefault`. A writable path is an `emptyDir`. `(enforced: PSA)`
- A component requiring privilege runs in its own namespace and never shares one with a `restricted` workload.
- Default-deny is enforced for ingress and egress across every platform pod; all allows are additive. `(enforced: CiliumNetworkPolicy)`
- WireGuard transparent encryption is on for all east-west pod traffic. Plaintext east-west is not shipped.
- A clusterwide policy denies `169.254.169.254/32`, so no egress grant can become a metadata-SSRF path. `(enforced: CiliumNetworkPolicy)`
- A new cluster bootstraps with the machine-config apply then the Argo CD root Application. There is no configuration-management step between them and no further manual steps.
- Growth beyond the day-one topology happens only on one of the documented triggers firing.
- No dedicated service mesh is deployed, sidecar or ambient. A mesh runs over the CNI rather than instead of it, so it is a second component re-providing encryption, identity, and L4 policy that Cilium already provides, and its L7 layer is already covered at the edge and in-app.
- Cilium NetworkPolicy is the internal service-to-service trust boundary, and each service declares its allowed callers. `(CI: lint:service-contract)`
