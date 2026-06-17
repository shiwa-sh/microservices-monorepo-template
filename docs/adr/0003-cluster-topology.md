# ADR-0003: Cluster Topology & Hosting

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0002](0002-monorepo.md), [ADR-0015](0015-naming-and-identifiers.md)

## Context

Three environments — **dev**, **staging**, **prod** — each is one cluster. Workloads include stateless application
services, stateful platform components (Postgres, Temporal, identity, observability), and ingress.

We need a single answer to:

- **Where production runs** — provider, machine class, failure characteristics.
- **Cluster shape** on day one and the path it grows along.
- **Traffic flow** from the public internet to a backend pod.
- **Storage** for stateful workloads.
- **How local development matches production.**

## Decision drivers

1. **Self-host** per [ADR-0000](0000-platform-foundations.md). No managed Kubernetes.
2. **Cost predictability** at 100-service scale. Per-cluster, per-LB, per-PV fees compound on managed K8s.
3. **Local–prod parity at the manifest layer.** Topology may differ; charts, code, and commands do not.
4. **Boring infrastructure.** Defaults are defaults for a reason. No exotic CNI, custom OS, or esoteric storage.
5. **Growth must follow measurable triggers**, not vibes.

## Decisions

### Hosting: cloud provider, chosen per project

Production runs on **compute instances from a cloud provider chosen per project** (e.g. Hetzner, GCP, or AWS — we run
k3s on plain compute instances, never the provider's managed Kubernetes), provisioned by Terraform under
`infra/terraform/`. The Terraform module isolates the provider behind a stable interface (instances, network, LB, DNS,
firewall, bucket); swapping to another provider is a module swap, not a topology change.

The cost of self-hosting is operational. Ansible roles under `infra/ansible/` are the codified operational knowledge:
new clusters are produced by `terraform apply` + `ansible-playbook bootstrap.yml` + `kubectl apply` of the ArgoCD root
Application ([ADR-0004](0004-gitops.md)).

### Distribution: k3s in production, k3d locally

`k3s` is the Kubernetes distribution: single binary, embedded etcd in HA mode, ships with Traefik / ServiceLB /
local-path / CoreDNS as replaceable defaults.

`k3d` is k3s in Docker, used locally. The same Helm charts and manifests apply.

### OS: Debian stable

The current Debian stable major on every node. Unattended-upgrades enabled for security patches. Kernel upgrades require
an explicit Ansible run with a cordoned reboot.

### Topology and growth triggers

**Day one (per environment):** three compute nodes running k3s with embedded etcd. All workloads — application services,
Postgres (via CNPG), Temporal, identity, observability — run on this 3-node set, sized for many cores and generous NVMe.

Three nodes from day one (not one) because:

- Embedded-etcd HA needs three nodes.
- A single-node cluster has multi-minute downtime on any node failure, which the platform thesis cannot accept even at
  the smallest scale.
- The cost difference (3× small machines vs 1× larger) is acceptable; the operational simplification (no "later, rebuild
  to HA" migration) is worth it.

**Growth triggers** — each tied to a measurable signal, each landing in a follow-up ADR when it fires:

| Trigger                | Signal                                                      | Response                                                                    |
|------------------------|-------------------------------------------------------------|-----------------------------------------------------------------------------|
| Resource pressure      | Sustained CPU or memory >70% for 7 days across the node set | Add worker nodes (k3s agents). Keep control plane at 3.                     |
| Storage scale          | Any service's PVC >50% of node disk                         | Adopt Longhorn as default storage class. Existing PVs migrate per-workload. |
| Compliance segregation | Regulated data with isolation requirement                   | Dedicated cluster for that workload.                                        |

Triggers are documented in `docs/cluster/growth-plan.md` so growth happens on data, not memory.

### Traffic flow

```text
Internet
  │
Provider Load Balancer  (provider L4 LB, one stable public IP per env)
  │
Traefik (k3s default)  (TLS termination via cert-manager + Let's Encrypt, L7 routing)
  ├── /api/*            ─▶ Tyk Gateway ─▶ backend service (per ADR-0009)
  ├── /internal/admin/* ─▶ Tyk Gateway ─▶ Lowdefy pod (internal admin, per ADR-0012)
  ├── /(landing|panel|admin|devportal)/* ─▶ Next.js frontend pod (one app, route groups per ADR-0014)
  └── /grafana/*        ─▶ Grafana (auth-gated)
```

**Traefik fronts Tyk, not the other way around.** Tyk is an API gateway: OpenAPI validation, JWT, rate limits. Traefik
is a cluster ingress: TLS, hostname routing, static assets. Mixing the roles couples deploy cadences.

**DNS:**

- One wildcard `*.<env>.example.com` `A` record per environment, pointing at the LB IP.
- `cert-manager` requests one wildcard certificate per environment via DNS-01 against the project's DNS provider.
- `external-dns` is not used. The wildcard absorbs new services.

**Cluster networking:** Cilium. Network policies are enabled cluster-wide with permissive defaults; per-service
tightening is part of the service template. Hubble (bundled) provides per-flow network visibility. k3s is installed
with `--flannel-backend=none --disable-network-policy`; Cilium is installed by the Ansible bootstrap role before
ArgoCD is started, then adopted by ArgoCD for upgrades.

### Storage

**Day one:**

- **Block storage:** k3s `local-path` provisioner. PVCs are node-local NVMe directories.
- **Object storage in production:** external S3-compatible bucket, chosen per environment in Terraform. Loki, Mimir,
  Tempo, Pyroscope, and CNPG backups all write here. **No MinIO in production**; offloading durability to a managed
  bucket eliminates an entire stateful component.
- **Object storage locally:** not installed by default; add a small MinIO manifest to `infra/local/deps.yaml` when a
  service under test needs object storage. Local-only.

When the storage-scale trigger fires, Longhorn becomes the default for new block PVCs; the external bucket strategy is
unchanged.

### Backups (mandatory, off-cluster)

- **CNPG `ScheduledBackup`** writes to the external bucket with WAL archiving for PITR. Retention: 30 days production, 7
  days non-prod.
- **Temporal history** lives on Postgres; covered by CNPG backups.
- **Observability long-term data** is already in the external bucket; the cluster PV holds hot cache only.
- **Node-level snapshots** via the cloud provider are taken daily as a catastrophic-recovery fallback.
- Backup restore is rehearsed quarterly as a Temporal `Schedule` ([ADR-0006](0006-temporal.md)) that opens a tracking
  issue.

### Provisioning order

```text
1. terraform apply              # compute instances, network, LB, DNS, firewall, bucket
2. ansible-playbook bootstrap   # OS hardening, kernel params, k3s install
3. kubectl apply -f infra/gitops/bootstrap/root-application.yaml
                                # ArgoCD reconciles the rest
```

The cluster identity is reproducible from git plus one Terraform state file (stored in the Terraform-managed bucket with
state locking).

### Local–prod parity

Parity is at the manifest, chart, and API level. Topology differences are explicit:

| Layer          | Local (k3d)     | Prod (k3s on cloud VMs)           | Same?         |
|----------------|-----------------|-----------------------------------|---------------|
| Kubernetes API | k3s             | k3s                               | yes           |
| Helm charts    | `infra/helm/`   | `infra/helm/`                     | yes           |
| Service code   | identical image | identical image                   | yes           |
| Ingress        | n/a (direct)    | Traefik                           | no, by design |
| TLS issuer     | n/a             | Let's Encrypt                     | no            |
| LB driver      | klipper-lb      | provider cloud-controller-manager | no            |
| Object storage | n/a (opt-in)    | external S3 bucket                | no, by design |
| GitOps         | not used        | ArgoCD                            | no, by design |
| Sizing         | tiny            | sized for traffic                 | no            |

`mise run dev:up` creates the k3d cluster and the lightweight dev dependencies; the inner loop is then driven by
Skaffold (`mise run dev`) — see *Local development* below. There is no docker-compose path: k3d is the single local
runtime, keeping local and prod on the same manifests.

### Local development

The local runtime is **k3d**; the inner loop is driven by **Skaffold**. The full platform is **not** installed locally —
that is ArgoCD's job in staging/prod ([ADR-0004](0004-gitops.md)). Locally you run the service(s) you are changing
against lightweight dependency stand-ins.

| Step         | Command                                 | Brings up                                                                                            |
|--------------|-----------------------------------------|------------------------------------------------------------------------------------------------------|
| Cluster+deps | `mise run dev:up`                       | k3d cluster + lightweight Postgres, Temporal dev server, in-memory SpiceDB (`infra/local/deps.yaml`) |
| Inner loop   | `mise run dev` (`skaffold dev`)         | builds each service image, deploys it via `infra/helm/service`, watches sources and live-rebuilds    |
| Debug        | `mise run dev:debug` (`skaffold debug`) | same, with Delve attached for IDE remote debugging                                                   |
| Migrations   | `mise run dev:migrate`                  | applies each service's migrations to the local Postgres                                              |
| Teardown     | `mise run dev:down`                     | deletes the cluster                                                                                  |

**Same chart, laptop knobs only.** Skaffold deploys the production `infra/helm/service` chart; the only overrides live
in `infra/helm/values/local-service.yaml` (ingress off, single replica, migrations run separately, endpoints pointed at
the local deps). Per-service `name`, image, and `DATABASE_URL` are injected by Skaffold. Adding a service to the loop is
one artifact + one release block in `skaffold.yaml`.

**Lightweight deps, not prod components.** `infra/local/deps.yaml` ships throwaway Postgres / Temporal-dev / in-memory
SpiceDB so a service has something to talk to. Their production counterparts (CNPG, the Temporal Helm chart,
operator-backed SpiceDB, the observability stack, the gateway and auth edge) are exercised by ArgoCD in a real staging
cluster, where their operators and ordering behave correctly. Add MinIO (or any other dep) to `deps.yaml` when a service
needs it.

**What is not swapped out, ever:** the Kubernetes API, the service chart, the service images, the env contract (
`DATABASE_URL`, `TEMPORAL_HOST_PORT`, OTLP, SpiceDB), the Postgres major version. A bug reproduced locally reproduces in
staging and prod. Skaffold loads images into k3d (no registry round-trip) and manages port-forwards.

### Service mesh

No dedicated service mesh (Istio, Linkerd, Consul Connect) is deployed. Sidecar meshes inject a proxy
container per pod — at 100 services that's 100+ extra containers on the hot path, against ADR-0000's per-service cost
principle — and the heavier ones (Istio, Consul Connect) add a CRD surface or a mandatory dependency the team size
cannot absorb.

**Cilium covers CNI + mesh as one component.** Its sidecarless eBPF mode provides mTLS (WireGuard node-to-node
encryption), L7 network policies, and per-flow observability (Hubble) without an injected proxy or a second
component. Cilium is installed from day one because CNI cannot be hot-swapped on a live cluster.

### Disaster recovery

Three-node HA tolerates single-node failure with no downtime; etcd quorum survives. A full-cluster loss recovers via
`terraform apply` → `ansible-playbook bootstrap` → ArgoCD reconciling from git → CNPG restoring Postgres from PITR.
Detection target <2 min (Uptime Kuma); recovery target <30 min; RPO ≈ WAL archive interval. Rehearsed quarterly
alongside the backup restore drill above.

## Consequences

### Positive

- Three-node HA from day one removes the "rebuild to HA later" migration entirely.
- Same k3s API end-to-end; local and prod differ in detail, not in shape.
- Growth triggers tied to measurable conditions, not opinion.
- External S3 for durable storage removes MinIO as a production component.
- Provisioning is reproducible from git + one Terraform state.

### Negative / Risks

- Three compute nodes cost more than one. Accepted; the alternative (later HA migration) is a maintenance window we
  never want to plan.
- k3s on bare metal is more ops than managed K8s. Mitigated by Ansible roles as the codified operational knowledge.
- Cilium is more complex to debug than Flannel (eBPF programs, `cilium status`, Hubble CLI). Mitigated by the Helm
  chart being committed and ArgoCD managing upgrades after the initial bootstrap.
- External bucket fees grow with retention. Mitigated by lifecycle policies (cold-tier after 30 days) configured in
  Terraform.

### Follow-ups

- `infra/terraform/modules/<provider>/` (e.g. `hetzner`) for compute instances, network, LB, DNS, firewall, bucket.
- `infra/ansible/roles/` for `k3s_server`, `cilium`, `hardening`, `unattended_upgrades`, `node_exporter`.
- `infra/helm/platform/{cilium,traefik,cert-manager,minio}/` with local and prod values.
- `docs/cluster/growth-plan.md` (triggers and responses).
- `docs/cluster/local-vs-prod.md` (parity table, divergences).
- `docs/cluster/dr-runbook.md` (full-cluster recovery).
- Quarterly DR drill as a Temporal `Schedule`.

## Rules

- Production runs on compute instances from a cloud provider chosen per project; provisioning is Terraform under
  `infra/terraform/`.
- Every environment runs k3s with three control-plane nodes (embedded etcd). Adding workers follows the
  resource-pressure trigger.
- Local development runs on k3d. The inner loop is Skaffold deploying the same `infra/helm/service` chart as production
  against lightweight deps (`infra/local/deps.yaml`); the full platform and ArgoCD are staging/prod-only.
- Ingress is Traefik with TLS via cert-manager. Tyk is an upstream service of Traefik, not the cluster ingress.
- Object storage in production is an external S3-compatible bucket. MinIO exists only in local development.
- Database backups are written off-cluster to the same external bucket and rehearsed quarterly.
- Storage class is k3s `local-path` until the storage-scale trigger fires, then Longhorn.
- CNI is Cilium from day one. k3s is installed with `--flannel-backend=none --disable-network-policy`. Cilium is
  bootstrapped by the Ansible `cilium` role (before ArgoCD) and adopted by ArgoCD for upgrades.
- A new cluster bootstraps with `terraform apply` → `ansible-playbook bootstrap` → `kubectl apply` of the ArgoCD root
  Application. No fourth manual step.
- Growth from day-one topology happens only on a documented trigger firing, captured in a new ADR.
- No dedicated service mesh is deployed. Sidecar meshes (Istio, Linkerd, Consul Connect) are ruled out by per-service
  resource cost and component count. Cilium covers CNI + zero-trust + L7 policies + Hubble observability in a single
  component with no per-pod proxy overhead.
