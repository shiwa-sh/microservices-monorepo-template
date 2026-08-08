# ADR-0200: Cluster Topology & Hosting

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0003](0003-naming-and-identifiers.md), [ADR-0201](0201-gitops.md), [ADR-0205](0205-environment-parity.md), [ADR-0302](0302-temporal.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0500](0500-observability.md), [ADR-0501](0501-operator-uis-and-dashboards.md)

## Context

Three environments — dev, staging, prod — are one cluster each. Workloads span stateless application services, stateful platform components, and ingress.

This ADR answers where production runs, what shape a cluster has on day one and how it grows, how traffic reaches a pod, and how storage and backups work. What may differ between environments is [ADR-0205](0205-environment-parity.md); the laptop is [ADR-0600](0600-local-development-loop.md).

## Decision drivers

1. **Self-host** ([ADR-0000](0000-platform-foundations.md), principle 3). No managed Kubernetes.
2. **Cost predictability across a growing fleet.** Per-cluster, per-load-balancer, and per-volume fees compound on managed Kubernetes.
3. **Parity at the manifest layer.** Topology may differ; charts, code, and commands do not.
4. **Boring infrastructure.** No exotic CNI, custom OS, or esoteric storage.
5. **Growth follows measurable triggers**, not judgement.

## Considered options

### East-west security, which is a CNI-level choice

Linkerd is not a CNI and not a Cilium substitute — it rides on top of a CNI through per-pod sidecars. The day-one decision is therefore at the CNI layer, compared on security capability.

| Capability | flannel only | flannel + Linkerd | **Cilium + WireGuard** |
| --- | --- | --- | --- |
| L3/L4 default-deny segmentation | **none — flat network** | meshed app traffic only | all pods |
| Data-tier protection (Postgres, MinIO, OpenFGA) | wide open | only if the data tier is meshed, which is fiddly | NetworkPolicy |
| Cryptographic workload identity | — | mTLS certs, meshed only | label identity; SPIFFE optional later |
| Encryption in transit, east-west | **plaintext** | meshed only | all pods, WireGuard |
| L7 authz by route and method | — | fine-grained | coarse, via Envoy — already covered by Oathkeeper and OpenFGA |
| Egress control, DNS/FQDN, metadata SSRF | — | not Linkerd's concern | FQDN and L3 egress |

**flannel silently ignores applied NetworkPolicy objects**, which is worse than having no policy because it grants false confidence. Between the two real options, Cilium wins on **breadth**: the controls it adds map onto the highest-frequency cluster attacks — lateral movement to Postgres, and metadata-endpoint credential theft. The controls Linkerd adds over Cilium are depth *behind* those, and the L7 layer is already covered at the edge and in-app. Linkerd would additionally leak the data tier unless the stateful components are meshed, which the Job-heavy bootstrap makes painful.

A sidecar mesh also adds one proxy container per pod on the hot path, against [ADR-0000](0000-platform-foundations.md)'s per-service cost principle.

### Kubernetes distribution

| Option | Verdict |
| --- | --- |
| **k3s** | **Chosen** — a single binary with embedded etcd in HA mode, shipping Traefik, ServiceLB, local-path, and CoreDNS as replaceable defaults |
| Managed Kubernetes (EKS, GKE, AKS) | Excluded by driver 1, and by per-cluster fees compounding across environments |
| Full upstream kubeadm | More moving parts to operate for no capability this platform uses |

### Day-one node count

| Option | Failure behaviour | Verdict |
| --- | --- | --- |
| **Three nodes, embedded etcd** | tolerates single-node loss with no downtime | **Chosen.** Embedded-etcd HA needs three, and this removes the later rebuild-to-HA migration entirely |
| One node | multi-minute downtime on any node failure | The thesis cannot accept that even at the smallest scale |

## Decision

### Hosting

Production runs on **plain compute instances**, never a provider's managed Kubernetes. How the instances come to exist is per project; two modes are supported against the same downstream bootstrap.

| Mode | Provisioning | Bucket |
| --- | --- | --- |
| Project provisions its own infrastructure | Terraform under `infra/terraform/` creates instances, network, LB, DNS, firewall, and bucket, isolating the provider behind a stable interface. Swapping providers is a module swap, not a topology change | created |
| Infrastructure is pre-provided | Terraform is skipped. The bootstrap runs against existing hosts named in a committed inventory | referenced by configuration |

**The dividing line is provisioning only.** Everything downstream — bootstrap, k3s, Cilium, Argo CD — is identical. Terraform is a per-project tool, not deployed or run by default.

The cost of self-hosting is operational, and the Ansible roles under `infra/ansible/` are that operational knowledge in code.

### Node OS

The current Debian stable major on every node, with unattended-upgrades enabled for security patches. A kernel upgrade requires an explicit run with a cordoned reboot.

### Topology and growth

Day one, per environment: three compute nodes running k3s with embedded etcd. All workloads run on this set, sized for many cores and generous NVMe.

Each growth trigger is tied to a measurable signal and lands in a follow-up ADR when it fires.

| Trigger | Signal | Response |
| --- | --- | --- |
| Resource pressure | sustained CPU or memory above 70% for 7 days across the node set | add worker agents; keep the control plane at three |
| Storage scale | any PVC exceeding 50% of node disk | adopt Longhorn as the default storage class; existing volumes migrate per workload |
| Compliance segregation | regulated data with an isolation requirement | a dedicated cluster for that workload |

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

Cilium is the CNI from day one. k3s is installed with `--flannel-backend=none --disable-network-policy`, and Cilium is installed by the bootstrap role before Argo CD starts, then adopted by Argo CD for upgrades. **CNI cannot be hot-swapped on a live cluster**, so the security posture is set at bootstrap rather than retrofitted.

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
| Block | k3s `local-path` — node-local NVMe directories | Longhorn becomes the default for new volumes |
| Object, production | an external S3-compatible bucket per environment. **No MinIO in production** | unchanged |
| Object, non-prod | in-cluster MinIO exposing the same S3 API ([ADR-0205](0205-environment-parity.md)) | unchanged |

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
0. terraform apply              # only when the project provisions its own infra
1. ansible-playbook bootstrap   # OS hardening, kernel params, k3s, Cilium
2. kubectl apply -f infra/gitops/bootstrap/root-application.yaml
                                # Argo CD reconciles the rest
```

Cluster identity is reproducible from git plus one Terraform state file when the project owns its infrastructure, or from git plus the committed inventory and the referenced bucket when it does not.

### Disaster recovery

Three-node HA tolerates single-node failure with no downtime; etcd quorum survives. A full-cluster loss recovers through `terraform apply` where applicable, then bootstrap, then Argo CD reconciling from git, then CNPG restoring from PITR. On pre-provided infrastructure the hosts already exist, so recovery starts at bootstrap.

| Target | Value |
| --- | --- |
| Detection | under 2 minutes |
| Recovery | under 30 minutes |
| RPO | the WAL archive interval |

Rehearsed quarterly alongside the backup restore drill.

## Consequences

### Positive

- Three-node HA from day one removes the rebuild-to-HA migration entirely.
- The same Kubernetes API end to end: environments differ in detail, not in shape.
- Growth triggers are tied to measurable conditions.
- An external bucket removes MinIO as a production component.
- Provisioning is reproducible from git, and Terraform is not a day-one dependency.

### Negative / Risks

- **Three nodes cost more than one.** Accepted: the alternative is a maintenance window nobody wants to plan.
- **k3s on plain compute is more operational work than managed Kubernetes.** Mitigated by the bootstrap roles being the codified knowledge.
- **Cilium is harder to debug than flannel** — eBPF programs, `cilium status`, the Hubble CLI. Mitigated by the chart being committed and Argo CD managing upgrades after bootstrap.
- **Bucket fees grow with retention.** Mitigated by lifecycle policies moving to a cold tier after 30 days.
- **CNI is set at bootstrap and cannot be changed live.** This is why the security posture is decided here rather than deferred.

### Follow-ups

- `infra/terraform/modules/<provider>/`, added only when a project provisions its own infrastructure.
- `infra/ansible/roles/` for `k3s_server`, `cilium`, `hardening`, `unattended_upgrades`, `node_exporter`, plus an inventory template for pre-provided hosts.
- `infra/helm/platform/{cilium,traefik,cert-manager,minio}/`.
- `docs/cluster/dr-runbook.md`.
- The quarterly DR drill as a Temporal `Schedule`.

## Rules

- Production runs on plain compute instances, never managed Kubernetes. Terraform is a per-project tool, skipped when infrastructure is pre-provided. `(review-only)`
- Every environment runs k3s with three control-plane nodes using embedded etcd. Adding workers follows the resource-pressure trigger. `(review-only)`
- Ingress is Traefik with TLS from cert-manager. Oathkeeper sits behind it as the edge identity filter; there is no API-management gateway. `(review-only)`
- Object storage in production is an external S3-compatible bucket. Non-prod uses in-cluster MinIO behind the same API. `(review-only)`
- Database backups are written off-cluster to that bucket and the restore is rehearsed quarterly. `(review-only)`
- The storage class is k3s `local-path` until the storage-scale trigger fires, then Longhorn. `(review-only)`
- CNI is Cilium from day one, bootstrapped before Argo CD and adopted by Argo CD for upgrades. `(review-only)`
- Default-deny is enforced for ingress and egress across every platform pod; all allows are additive. `(enforced: CiliumNetworkPolicy)`
- WireGuard transparent encryption is on for all east-west pod traffic. Plaintext east-west is not shipped. `(review-only)`
- A clusterwide policy denies `169.254.169.254/32`, so no egress grant can become a metadata-SSRF path. `(enforced: CiliumNetworkPolicy)`
- A new cluster bootstraps with the provisioning play then the Argo CD root Application. There are no further manual steps. `(review-only)`
- Growth beyond the day-one topology happens only on a documented trigger firing, captured in a new ADR. `(review-only)`
- No dedicated service mesh is deployed. Sidecar meshes are ruled out by per-service resource cost and component count. `(review-only)`
- Cilium NetworkPolicy is the internal service-to-service trust boundary, and each service declares its allowed callers. `(CI: lint:service-contract)`
