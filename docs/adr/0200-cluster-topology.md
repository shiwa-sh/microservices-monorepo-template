# ADR-0200: Cluster Topology & Hosting

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0104](0104-supply-chain-security.md), [ADR-0201](0201-gitops.md), [ADR-0203](0203-policy-enforcement.md), [ADR-0205](0205-environment-parity.md), [ADR-0206](0206-cluster-networking.md), [ADR-0207](0207-cluster-storage.md)
- **Decides:** Production runs upstream Kubernetes on Talos, three nodes per environment on plain compute instances the project may or may not provision.

## Context

Three environments — dev, staging, prod — are one cluster each. Workloads span stateless application services, stateful platform components, and ingress.

This ADR answers where production runs, what a node is, what shape a cluster has on day one, how it grows, and what recovering it costs.

Two questions settled at the same bootstrap have their own decisions, because each is load-bearing for readers who never touch provisioning: the pod network is [ADR-0206](0206-cluster-networking.md), and volumes and backups are [ADR-0207](0207-cluster-storage.md). What may differ between environments is [ADR-0205](0205-environment-parity.md); the laptop is [ADR-0600](0600-local-development-loop.md).

## Decision drivers

1. **Operational sovereignty** ([ADR-0000](0000-platform-foundations.md), principle 3). Nobody outside the organisation can change the terms on which the control plane runs, and per-cluster, per-load-balancer and per-volume fees do not compound across a growing fleet.
2. **A node matches its description.** The gap between what a host is declared to be and what it has become is a failure class in its own right, and one this layer can remove rather than monitor.
3. **Boring by default, novel where it removes a class of failure** ([ADR-0000](0000-platform-foundations.md), *spend novelty by exit cost*). A less widely operated component is adopted where it eliminates a category of failure rather than improving on one, and the ADR names the category.
4. **Losing one node is not downtime.**

Manifest-layer parity is a constraint inherited from [ADR-0205](0205-environment-parity.md) rather than a driver here: topology may differ between environments, charts and commands may not.

## Considered options

### The orchestrator itself

Kubernetes is Tier 1 ([ADR-0002](0002-tool-adoption.md)): every chart, policy, operator, and the whole GitOps layer are expressed in its API. Assuming it is what produces a platform nobody can justify the weight of.

| Option | Ecosystem this platform depends on | Operational weight | What replaces the API | Verdict |
| --- | --- | --- | --- | --- |
| **Upstream Kubernetes** | CNPG, Cilium, cert-manager, Argo CD, Kyverno, and every chart on the floor exist because of it *(reasoned)* | high, and it is the dominant term in [`../operational-surface.md`](../operational-surface.md) | — | **Chosen.** The floor is not Kubernetes; the floor is the operators that only exist on it, and each would otherwise be a first-party system |
| Nomad | scheduling and secrets are covered; **the stateful operators are not** — there is no CNPG equivalent, so Postgres HA, backup scheduling, and PITR become first-party work *(documented)* | materially lower — one binary, and no CRD layer | Nomad job files, plus Consul for service discovery | **The runner-up.** It wins on weight and loses on the row that matters most at axis C high: the database. Reopens if the estate stops needing an operator ecosystem |
| Docker Swarm | none of it | lowest of the orchestrators | compose files | Effectively unmaintained as a growing platform, and no policy or admission layer at all |
| systemd units on plain instances | none | lowest overall, and highest per-service | provisioning, plus a service discovery mechanism to write | The honest baseline. It is the correct answer at axis A low; at axis A high the per-service cost compounds into the coordination problem the orchestrator exists to solve |
| A self-hosted PaaS — Dokku, CapRover, Coolify | none | low | the PaaS's own model | Each is an abstraction over containers with its own opinionated deploy path, and none carries a stateful-workload operator model. Principle 1 also fails: their state is a database and their UI is the editor |
| Managed Kubernetes | all of it | the provider's | — | Fails principle 3. Recorded here because it is the option most reference architectures at this position take |

**The decisive row is stateful workloads.** At axis C high, failover, backup, and point-in-time recovery are the platform's core obligation rather than a component choice, and CloudNativePG is what expresses that obligation as four CRDs instead of a runbook and a cron job. Nomad's weight advantage is real, and is spent buying exactly that back in first-party code.

### Node operating system

| Option | Shell and SSH on the node | Configuration | Kubernetes | Verdict |
| --- | --- | --- | --- | --- |
| **Talos Linux** | **neither — an API is the only interface** | a declarative machine config document applied over gRPC | upstream, shipped and upgraded with the OS | **Chosen** *(reasoned)* |
| Flatcar Container Linux | retained, and some two thousand binaries with them | Ignition at provision time, and whatever mutates the host afterwards | installed separately | The CoreOS lineage, and it keeps the shell and the drift a shell permits. Only `/usr` is read-only |
| Fedora CoreOS | retained | Ignition and rpm-ostree | installed separately | As Flatcar, and it does not assume Kubernetes, so the parts this platform never uses are still attack surface |
| Bottlerocket | disabled by default, reachable through an admin container | API-driven, closest in spirit to Talos | installed separately | The nearest philosophical match. Its platform support is AWS-centric and bare metal is poorly documented, which driver 1 makes decisive |
| Debian stable, converged by Ansible | full | playbooks converging a mutable host | k3s, installed by playbook | The honest baseline. A node matches its description only to the degree the playbooks are complete, and re-running them is the only evidence |

**The node is the only mutable thing left in the system.** Cluster state is reconciled from git ([ADR-0201](0201-gitops.md)), pods run on a base with no shell and no package manager ([ADR-0101](0101-monorepo.md)), and the pod network is default-deny. Against that posture the host shell is the one remaining escalation path and the one remaining source of drift, and an immutable node collapses the description and the thing described into a single object. That is the failure class driver 3 requires naming.

Driver 3 also admits the cost: this is a less widely operated OS than Debian, and the ADR pays for it in *Consequences* rather than pretending otherwise.

### Kubernetes distribution

| Option | Installer to operate | Coupling to the node OS | What it bundles | Verdict |
| --- | --- | --- | --- | --- |
| **Upstream Kubernetes, shipped by Talos** | none | one artefact, one upgrade path | nothing beyond Kubernetes | **Chosen.** The distribution and the OS upgrade together, and there is no installer standing between them *(reasoned)* |
| k3s | its own | independent | Traefik, ServiceLB, `local-path`, CoreDNS, all replaceable | Those bundles are the real loss here, and each is replaceable by a chart this repository already commits. Its lighter footprint belongs to the single-node SQLite configuration, which three-node HA forecloses by requiring etcd. **Resources decide this row in neither direction**, and it is conformant, so it is not a smaller Kubernetes to graduate from later |
| k0s | its own | independent | nothing | k3s without the bundles, which removes the only real distinction from the chosen option while keeping the separate installer |
| Full upstream kubeadm | its own | independent | nothing | The same upstream Kubernetes with an installer to operate, which is the part Talos removes |
| Managed Kubernetes (EKS, GKE, AKS) | none — the provider's | none | the provider's opinions | Fails driver 1: the terms on which the control plane runs are the provider's, and per-cluster fees compound across environments |

### Day-one node count

| Option | Failure behaviour | Migration to HA later | Verdict |
| --- | --- | --- | --- |
| **Three nodes, embedded etcd** | tolerates single-node loss with no downtime | none needed | **Chosen.** Embedded-etcd quorum needs three, and starting there removes the rebuild entirely *(reasoned)* |
| One node | multi-minute downtime on any node failure | a rebuild, with a data migration | Fails driver 4 at every scale |
| Two nodes | **worse than one** — no quorum, and two failure domains to lose it in | a rebuild | Even quorum is the classic mistake this row exists to name |
| Three nodes, etcd on dedicated hosts | as chosen | none | The same guarantee for more machines. It becomes right when etcd contends with workloads, which is a growth trigger rather than a day-one shape |

### Provisioning

| Option | Licence | State model | Provider coverage for this shape | Verdict |
| --- | --- | --- | --- | --- |
| **Terraform** | [BUSL-1.1](https://www.hashicorp.com/license-faq) *(documented)* | one state file per environment, held with the project's infrastructure | the `siderolabs/talos` provider is first-party, which is what makes machine-config apply and bootstrap a plan rather than a script *(documented)* | **Chosen**, and it is skipped entirely where infrastructure is pre-provided — so the licence binds a tool a project may never run |
| OpenTofu | MPL-2.0 | identical | the same providers, and its own registry | The fork exists precisely because of the licence above, and it is a drop-in. It is the standing exit rather than the choice: the deciding factor is that the Talos provider's releases target the incumbent first, and this is the one provider whose currency is load-bearing |
| Pulumi | Apache-2.0 | a service or a self-managed backend | good | Infrastructure in a general-purpose language, which is more expressive than this shape needs and adds a backend to run or a service to depend on |
| Crossplane | Apache-2.0 | Kubernetes resources, reconciled | good | The cluster provisions its own substrate, which is a bootstrap circularity: the thing being created is where the creator runs |
| Provider CLIs in a script | — | none | total | The honest baseline. It has no plan step, so the difference between intent and effect is discovered by running it |

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

This is narrower than it sounds against [12-Factor XII](https://12factor.net/admin-processes): one-off admin work still runs, in the same image and release as the service — migrations as an init container ([ADR-0300](0300-data.md)), operator tasks through the admin UIs ([ADR-0401](0401-internal-admin.md)). What is unavailable is the host shell, not the one-off process.

### Topology and growth

Day one, per environment: three compute nodes running Talos, with etcd on all three. All workloads run on this set, sized for many cores and generous NVMe.

Each growth step is a deferral, and carries all three fields. The storage-scale trigger is a third, and it belongs to [ADR-0207](0207-cluster-storage.md) because its seam is an OS-image change rather than a node count.

| Trigger | Response | Seam | Cost if adopted late |
| --- | --- | --- | --- |
| Sustained CPU or memory above 70% for 7 days across the node set | add worker agents; the control plane stays at three | ✓ a worker's machine config is a committed document and joining is an apply. No workload is pinned to a node | capacity is added under pressure rather than ahead of it, so the window is an incident rather than a change |
| Regulated data carrying an isolation requirement | a dedicated cluster for that workload | ✓ every environment is already one cluster built from the same committed configuration, so another is a values selection ([ADR-0205](0205-environment-parity.md)) | the regulated data has already been co-resident, which no later separation undoes |

### Workload hardening

Talos removes the host attack surface and Cilium removes the network's. Neither constrains what a *pod* may ask the kernel for, which is the third invariant.

**Every namespace enforces the [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/) `restricted` profile**, through the in-tree Pod Security Admission controller — namespace labels, no component:

```yaml
pod-security.kubernetes.io/enforce: restricted
pod-security.kubernetes.io/enforce-version: <cluster minor>
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

Three-node HA tolerates single-node failure with no downtime; etcd quorum survives.

A full-cluster loss recovers through `terraform apply` where applicable, then machine-config apply and bootstrap, then Argo CD reconciling from git, then CNPG restoring from PITR. On pre-provided infrastructure the Talos nodes already exist, so recovery starts at the machine-config apply.

The recovery objectives are decided here rather than in the runbook that executes them, and they bind [ADR-0207](0207-cluster-storage.md)'s backup retention, Object Lock window, and rehearsal cadence.

| Objective | Value | What sets it |
| --- | --- | --- |
| **RPO** — data loss tolerated | the WAL archive interval | CNPG's archiving cadence. Lowering it costs archive volume, not architecture |
| **RTO** — restored and serving | under 30 minutes from the start of recovery | the four-step path above, where reconciliation from git dominates |
| **Time to start recovering** | in hours, minutes; out of hours, next working day | nothing pages ([ADR-0502](0502-alerting-and-on-call.md)). The composed figure is [`reference/detection-latency.md`](../reference/detection-latency.md) |

**Detection is outside RTO, and an availability objective is bounded by both rows.** A recovery objective stated without that boundary is one the platform cannot honour out of hours, because a user's clock starts at the failure rather than at the response.

Rehearsed quarterly alongside the backup restore drill ([ADR-0207](0207-cluster-storage.md)), which is what makes these values measurements rather than intentions.

## Consequences

### Positive

- Three-node HA from day one removes the rebuild-to-HA migration entirely.
- **Node configuration drift is not mitigated, it is impossible.** The machine config is the node, and there is no interface through which the two can diverge.
- **The node has no shell for a compromised process to reach**, which is the posture the pods already had, now extended to the host under them.
- An OS upgrade is an image swap with a rollback rather than a package transaction with a partial-failure mode.
- The same Kubernetes API end to end: environments differ in detail, not in shape.
- Growth triggers are tied to measurable conditions.
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

## Rules

- Production runs on plain compute instances, never managed Kubernetes. Terraform is a per-project tool, skipped when infrastructure is pre-provided.
- Every node runs Talos Linux, configured only by its machine config. There is no SSH, no configuration-management agent, and no manual change to a node.
- Every environment runs three control-plane nodes with etcd on each. Adding workers follows the resource-pressure trigger.
- Anything not in the base Talos image arrives as a system extension in a pinned installer image built through Image Factory.
- Every namespace carries the Pod Security Standards `restricted` labels with a pinned `enforce-version`. A namespace at `baseline` or `privileged` records why in its manifest. `(ref: Pod Security Standards; enforced: PSA)`
- Service containers run as non-root with a read-only root filesystem, `ALL` capabilities dropped, `allowPrivilegeEscalation: false`, and `seccompProfile: RuntimeDefault`. A writable path is an `emptyDir`. `(enforced: PSA)`
- A component requiring privilege runs in its own namespace and never shares one with a `restricted` workload.
- A new cluster bootstraps with the machine-config apply then the Argo CD root Application. There is no configuration-management step between them and no further manual steps.
- Growth beyond the day-one topology happens only on one of the documented triggers firing.
