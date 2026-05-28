# ADR-0003: Cluster Topology & Hosting

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0002](0002-monorepo.md), [ADR-0015](0015-naming-and-identifiers.md)

## Context

Three environments — **dev**, **staging**, **prod** — each is one cluster. Workloads include stateless application services, stateful platform components (Postgres, Temporal, identity, observability), and ingress.

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

### Hosting: Hetzner Cloud

Production runs on **Hetzner Cloud VPS instances**, provisioned by Terraform under `infra/terraform/`. Hetzner is chosen for cost per core and integration via `hcloud-cloud-controller-manager`. The Terraform module isolates the provider; switching to an equivalent (OVH, Latitude.sh) is a module swap.

The cost of self-hosting is operational. Ansible roles under `infra/ansible/` are the codified operational knowledge: new clusters are produced by `terraform apply` + `ansible-playbook bootstrap.yml` + `kubectl apply` of the ArgoCD root Application ([ADR-0004](0004-gitops.md)).

The template can target **another cloud provider** when a project requires it; the Terraform module swap above covers the provider, and instance/resource names follow [ADR-0015](0015-naming-and-identifiers.md) regardless of cloud.

### Distribution: k3s in production, k3d locally

`k3s` is the Kubernetes distribution: single binary, embedded etcd in HA mode, ships with Traefik / ServiceLB / local-path / CoreDNS as replaceable defaults.

`k3d` is k3s in Docker, used locally. The same Helm charts and manifests apply.

### OS: Ubuntu LTS

The current Ubuntu LTS major on every node. Unattended-upgrades enabled for security patches. Kernel upgrades require an explicit Ansible run with a cordoned reboot.

### Topology and growth triggers

**Day one (per environment):** three VPS nodes running k3s with embedded etcd. All workloads — application services, Postgres (via CNPG), Temporal, identity, observability — run on this 3-node set, sized for many cores and generous NVMe.

Three nodes from day one (not one) because:

- Embedded-etcd HA needs three nodes.
- A single-node cluster has multi-minute downtime on any node failure, which the platform thesis cannot accept even at the smallest scale.
- The cost difference (3× small machines vs 1× larger) is acceptable; the operational simplification (no "later, rebuild to HA" migration) is worth it.

**Growth triggers** — each tied to a measurable signal, each landing in a follow-up ADR when it fires:

| Trigger                | Signal                                                      | Response                                                                    |
|------------------------|-------------------------------------------------------------|-----------------------------------------------------------------------------|
| Resource pressure      | Sustained CPU or memory >70% for 7 days across the node set | Add worker nodes (k3s agents). Keep control plane at 3.                     |
| Storage scale          | Any service's PVC >50% of node disk                         | Adopt Longhorn as default storage class. Existing PVs migrate per-workload. |
| Network policy needs   | Need for eBPF observability or zero-trust policies          | Swap Flannel → Cilium at next cluster rebuild.                              |
| Compliance segregation | Regulated data with isolation requirement                   | Dedicated cluster for that workload.                                        |

Triggers are documented in `docs/cluster/growth-plan.md` so growth happens on data, not memory.

### Traffic flow

```text
Internet
  │
Hetzner Load Balancer  (provider L4 LB, one stable public IP per env)
  │
Traefik (k3s default)  (TLS termination via cert-manager + Let's Encrypt, L7 routing)
  ├── /api/*       ─▶ Tyk Gateway  ─▶ backend service (per ADR-0009)
  ├── /panel/*     ─▶ Next.js pod
  ├── /landing/*   ─▶ Next.js pod
  ├── /admin/*     ─▶ Next.js pod
  ├── /devportal/* ─▶ Next.js pod
  └── /grafana/*   ─▶ Grafana (auth-gated)
```

**Traefik fronts Tyk, not the other way around.** Tyk is an API gateway: OpenAPI validation, JWT, rate limits. Traefik is a cluster ingress: TLS, hostname routing, static assets. Mixing the roles couples deploy cadences.

**DNS:**

- One wildcard `*.<env>.example.com` `A` record per environment, pointing at the LB IP.
- `cert-manager` requests one wildcard certificate per environment via DNS-01 (Cloudflare).
- `external-dns` is not used. The wildcard absorbs new services.

**Cluster networking:** Flannel + VXLAN. Network policies are enabled cluster-wide with permissive defaults; per-service tightening is part of the service template.

### Storage

**Day one:**

- **Block storage:** k3s `local-path` provisioner. PVCs are node-local NVMe directories.
- **Object storage in production:** external S3-compatible bucket (Cloudflare R2 or AWS S3 — chosen per environment in Terraform). Loki, Mimir, Tempo, Pyroscope, and CNPG backups all write here. **No MinIO in production**; offloading durability to a managed bucket eliminates an entire stateful component.
- **Object storage locally:** small MinIO Helm install for `mise run dev:up` (k3d). Local-only.

When the storage-scale trigger fires, Longhorn becomes the default for new block PVCs; the external bucket strategy is unchanged.

### Backups (mandatory, off-cluster)

- **CNPG `ScheduledBackup`** writes to the external bucket with WAL archiving for PITR. Retention: 30 days production, 7 days non-prod.
- **Temporal history** lives on Postgres; covered by CNPG backups.
- **Observability long-term data** is already in the external bucket; the cluster PV holds hot cache only.
- **Node-level snapshots** via Hetzner are taken daily as a catastrophic-recovery fallback.
- Backup restore is rehearsed quarterly as a Temporal `Schedule` ([ADR-0006](0006-temporal.md)) that opens a tracking issue.

### Provisioning order

```text
1. terraform apply              # VPS instances, network, LB, DNS, firewall, bucket
2. ansible-playbook bootstrap   # OS hardening, kernel params, k3s install
3. kubectl apply -f infra/gitops/bootstrap/root-application.yaml
                                # ArgoCD reconciles the rest
```

The cluster identity is reproducible from git plus one Terraform state file (stored in the Terraform-managed bucket with state locking).

### Local–prod parity

Parity is at the manifest, chart, and API level. Topology differences are explicit:

| Layer          | Local (k3d)     | Prod (k3s on Hetzner) | Same?                     |
|----------------|-----------------|-----------------------|---------------------------|
| Kubernetes API | k3s             | k3s                   | yes                       |
| Helm charts    | `infra/helm/`   | `infra/helm/`         | yes                       |
| Service code   | identical image | identical image       | yes                       |
| Ingress        | Traefik         | Traefik               | yes                       |
| TLS issuer     | mkcert local CA | Let's Encrypt         | no                        |
| LB driver      | klipper-lb      | hcloud-ccm            | no                        |
| Object storage | MinIO           | external S3 bucket    | interface yes, backend no |
| GitOps         | not used        | ArgoCD                | no, by design             |
| Sizing         | tiny            | sized for traffic     | no                        |

`mise run dev:up` brings up the full k3d cluster with `infra/helm/` charts and local-built images. For fast inner-loop work, `mise run dev:up --minimal` boots a stripped k3d profile (Postgres, Temporal dev server, OTel-LGTM bundle, MinIO) using a reduced Helm values overlay. There is no docker-compose path: k3d is the single local runtime, keeping local and prod on the same manifests.

### Local development

k3d is the single local runtime. Two profiles, one cluster lifecycle:

| Profile | Command                     | Brings up                                                                    | Use for                                                       |
|---------|-----------------------------|------------------------------------------------------------------------------|---------------------------------------------------------------|
| Full    | `mise run dev:up`           | every chart in `infra/helm/` (services, gateway, auth, observability, MinIO) | end-to-end flows, gateway/auth/policy work, demo              |
| Minimal | `mise run dev:up --minimal` | Postgres (CNPG), Temporal dev server, OTel-LGTM bundle, MinIO                | inner loop: running one service + its tests against real deps |

Both profiles use the same charts. Minimal is a values overlay (`infra/helm/values/local-minimal.yaml`) that:

- Sets `replicas: 1` everywhere and removes `PodDisruptionBudget`s and anti-affinity.
- Disables ArgoCD, cert-manager, Renovate webhooks, backups, alerting, and long-term observability storage (Loki/Mimir/Tempo run with in-memory retention only).
- Disables the gateway, auth stack (Kratos/Hydra/SpiceDB), and any non-target services. Services under test run directly via `mise run run` in `services/<name>/` and talk to deps over `localhost` port-forwards established by `dev:up`.
- Substitutes `local-path` for the production storage class and shrinks PVC requests.

What is **not** swapped out, ever: the Kubernetes API, the chart structure, the service images, the OTLP endpoint shape, the Postgres major version. A bug that reproduces against minimal reproduces against full and against prod.

**Cold-start budget.** Full profile boots in <90s on a developer laptop; minimal in <20s. If either regresses, it gets treated as a build-time regression and fixed — slow `dev:up` is the failure mode that pushes engineers back toward ad-hoc compose files and breaks the single-source-of-truth invariant.

**Port-forwards are declarative.** `infra/helm/values/local-minimal.yaml` carries the canonical port map (Postgres 5432, Temporal 7233, OTLP 4317, Grafana 3000, MinIO 9000/9001). `mise run dev:up` establishes them via `kubectl port-forward` managed as background tasks; `mise run dev:down` tears them down. Services running on the host talk to deps at `localhost:<canonical-port>` with no per-developer config.

**Image loading.** `mise run dev:build <service>` builds the image and `k3d image import`s it into the cluster, then triggers a `kubectl rollout restart`. No registry round-trip.

### Disaster recovery

Three-node HA tolerates single-node failure with no downtime; etcd quorum survives. A full-cluster loss is the disaster case:

1. **Detection** within 1–2 minutes via Uptime Kuma (self-hosted) paging on-call.
2. **Recovery (target <30 min):**
   - `terraform apply` provisions a new node set.
   - `ansible-playbook bootstrap` installs k3s.
   - ArgoCD root Application reconciles every component from git.
   - CNPG restores Postgres from PITR in the external bucket.
3. **RPO ≈ WAL archive interval** (minutes). In-flight requests at the moment of failure are lost.
4. **Rehearsed quarterly** against a staging rebuild, tracked as a Temporal `Schedule`.

## Consequences

### Positive

- Three-node HA from day one removes the "rebuild to HA later" migration entirely.
- Same k3s API end-to-end; local and prod differ in detail, not in shape.
- Growth triggers tied to measurable conditions, not opinion.
- External S3 for durable storage removes MinIO as a production component.
- Provisioning is reproducible from git + one Terraform state.

### Negative / Risks

- Three Hetzner nodes cost more than one. Accepted; the alternative (later HA migration) is a maintenance window we never want to plan.
- k3s on bare metal is more ops than managed K8s. Mitigated by Ansible roles as the codified operational knowledge.
- Flannel + permissive policies on day one is not zero-trust. Tightening per-service is part of the service template; Cilium is the documented upgrade.
- External bucket fees grow with retention. Mitigated by lifecycle policies (cold-tier after 30 days) configured in Terraform.

### Follow-ups

- `infra/terraform/modules/hetzner/` for VPS, network, LB, DNS, firewall, bucket.
- `infra/ansible/roles/` for `k3s_server`, `hardening`, `unattended_upgrades`, `node_exporter`.
- `infra/helm/platform/{traefik,cert-manager,minio}/` with local and prod values.
- `docs/cluster/growth-plan.md` (triggers and responses).
- `docs/cluster/local-vs-prod.md` (parity table, divergences).
- `docs/cluster/dr-runbook.md` (full-cluster recovery).
- Quarterly DR drill as a Temporal `Schedule`.

## Rules

- Production runs on Hetzner Cloud VPS instances; provisioning is Terraform under `infra/terraform/`.
- Every environment runs k3s with three control-plane nodes (embedded etcd). Adding workers follows the resource-pressure trigger.
- Local development runs k3d using the same Helm charts as production.
- Ingress is Traefik with TLS via cert-manager. Tyk is an upstream service of Traefik, not the cluster ingress.
- Object storage in production is an external S3-compatible bucket. MinIO exists only in local development.
- Database backups are written off-cluster to the same external bucket and rehearsed quarterly.
- Storage class is k3s `local-path` until the storage-scale trigger fires, then Longhorn.
- CNI is Flannel until the network-policy trigger fires, then Cilium.
- A new cluster bootstraps with `terraform apply` → `ansible-playbook bootstrap` → `kubectl apply` of the ArgoCD root Application. No fourth manual step.
- Growth from day-one topology happens only on a documented trigger firing, captured in a new ADR.
