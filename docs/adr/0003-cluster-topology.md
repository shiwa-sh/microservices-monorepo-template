# ADR-0003: Cluster Topology & Hosting

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:
  ** [ADR-0000](0000-platform-foundations.md), [ADR-0002](0002-monorepo.md), [ADR-0015](0015-naming-and-identifiers.md)

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

### Hosting: compute instances, provisioned per project

Production runs on **plain compute instances** (e.g. Hetzner, GCP, or AWS — we run k3s on compute instances, never the
provider's managed Kubernetes). How those instances come to exist is **per project**, and the template supports two
modes against the same downstream bootstrap:

- **Project provisions its own infrastructure** — Terraform under `infra/terraform/` creates instances, network, LB,
  DNS, firewall, and bucket, isolating the provider behind a stable interface; swapping providers is a module swap, not
  a
  topology change. Terraform is a per-project tool, **not deployed or run by default**: it is added when a project owns
  its infrastructure, exactly like the other latent tools in the toolchain.
- **Infrastructure is pre-provided** — many projects deploy onto compute, network, and storage the operator already
  owns. Here Terraform is skipped entirely; the Ansible bootstrap runs against the existing hosts (named in an
  inventory)
  and the bucket is referenced by configuration rather than created.

The dividing line is provisioning only. Everything downstream — Ansible bootstrap, k3s, Cilium, ArgoCD — is identical in
both modes.

The cost of self-hosting is operational. Ansible roles under `infra/ansible/` are the codified operational knowledge:
the universal path is `ansible-playbook bootstrap.yml` + `kubectl apply` of the ArgoCD root Application
([ADR-0004](0004-gitops.md)), preceded by `terraform apply` only when the project provisions its own infrastructure.

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
Traefik (k3s default)  (TLS termination via cert-manager + Let's Encrypt, L7 routing, rate limiting)
  ├── /api/*            ─▶ Oathkeeper (identity) ─▶ backend service (per ADR-0009)
  ├── /internal/admin/* ─▶ Oathkeeper (identity) ─▶ Lowdefy pod (internal admin, per ADR-0012)
  ├── /(landing|panel|admin|devportal)/* ─▶ Next.js frontend pod (one app, route groups per ADR-0014)
  ├── /grafana/*        ─▶ Grafana (auth-gated)
  └── hubble.<host>/    ─▶ Hubble UI (Cilium network / service-map dashboard, auth-gated; own subdomain at root — its router can't run under a path prefix)
```

**Traefik is the only ingress; Oathkeeper is an auth filter behind it, not a second gateway.** Traefik does TLS,
hostname/path routing, load balancing, and rate limiting; Ory Oathkeeper validates identity and injects identity headers
([ADR-0009](0009-api-gateway.md)). There is no API-management gateway in the default stack.

**DNS:**

- One wildcard `*.<env>.example.com` `A` record per environment, pointing at the LB IP.
- `cert-manager` requests one wildcard certificate per environment via DNS-01 against the project's DNS provider.
- `external-dns` is not used. The wildcard absorbs new services.

**Cluster networking:** Cilium. Network policies are the platform's internal service-to-service trust boundary
([ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md)): the default is **deny**, and each service's chart declares
which callers may reach it. Because internal calls carry forwarded identity headers and no token, NetworkPolicy is what
guarantees only sanctioned callers reach a service's port. Hubble (bundled, UI exposed auth-gated at the `hubble.<host>`
subdomain) provides per-flow visibility and is the audit surface for these policies. k3s is installed with
`--flannel-backend=none --disable-network-policy`; Cilium is installed by the Ansible bootstrap role before ArgoCD is
started, then adopted by ArgoCD for upgrades.

### Storage

**Day one:**

- **Block storage:** k3s `local-path` provisioner. PVCs are node-local NVMe directories.
- **Object storage in production:** external S3-compatible bucket, per environment — created by Terraform when the
  project provisions its own infra, or referenced by configuration when the bucket is pre-provided. Loki, Mimir,
  Tempo, CNPG backups (and Pyroscope where profiling is enabled, [ADR-0011](0011-observability.md)) all write here.
  **No MinIO in production**; offloading durability to a managed
  bucket eliminates an entire stateful component.
- **Object storage in non-prod:** in-cluster MinIO (`infra/helm/platform/minio`), exposing the same S3 API as the prod
  bucket. It runs in the full-platform local tier and in dev/staging; the inner loop omits it unless a service under
  test needs it ([ADR-0016](0016-environment-parity.md)).

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
0. terraform apply              # ONLY when the project provisions its own infra:
                                # compute instances, network, LB, DNS, firewall, bucket
1. ansible-playbook bootstrap   # OS hardening, kernel params, k3s install (runs against the
                                # Terraform-produced hosts, or a hand-written inventory of pre-provided hosts)
2. kubectl apply -f infra/gitops/bootstrap/root-application.yaml
                                # ArgoCD reconciles the rest
```

When the project provisions its own infra, the cluster identity is reproducible from git plus one Terraform state file
(stored in the Terraform-managed bucket with state locking). When infra is pre-provided, the same reproducibility comes
from git plus the committed Ansible inventory and the referenced bucket; there is no Terraform state to keep.

### Local–prod parity

Parity is at the manifest, chart, and API level. Topology differences are explicit:

| Layer          | Local (k3d)     | Prod (k3s on cloud VMs)           | Same?         |
|----------------|-----------------|-----------------------------------|---------------|
| Kubernetes API | k3s             | k3s                               | yes           |
| Helm charts    | `infra/helm/`   | `infra/helm/`                     | yes           |
| Service code   | identical image | identical image                   | yes           |
| Ingress        | inner: direct / full: Traefik | Traefik             | full tier: yes |
| TLS issuer     | cert-manager (self-signed) | cert-manager (Let's Encrypt) | mechanism: yes |
| LB driver      | klipper-lb      | provider cloud-controller-manager | no            |
| Object storage | MinIO (non-prod) | external S3 bucket               | API: yes      |
| GitOps         | inner: n/a / full: ArgoCD | ArgoCD                  | full tier: yes |
| Sizing         | tiny            | sized for traffic                 | no            |

`mise run cluster:lite` creates the k3d cluster and the lightweight dev dependencies; the inner loop is then **native
execution** — you run the service you are changing directly on the host (any editor/IDE, or `go run`) against those
dependencies — see *Local development* below. There is no docker-compose path: k3d is the single local runtime, keeping
local and prod on the same manifests.

### Local development

The local runtime is **k3d**, in two tiers ([ADR-0016](0016-environment-parity.md)). The **inner loop** below runs the
service you are changing **natively on the host** against lightweight dependency stand-ins reached via port-forwards —
the day-to-day path: no image build, no in-cluster redeploy, no file-watch on the hot path. The **full platform**
(`mise run cluster:full`) brings the real charts up at a single replica, delivered by **ArgoCD** — the same mechanism
the persistent dev/staging/prod clusters use ([ADR-0004](0004-gitops.md)) — for end-to-end and pre-merge validation.

| Step          | Command                                    | Brings up / does                                                                                     |
|---------------|--------------------------------------------|------------------------------------------------------------------------------------------------------|
| Cluster+deps  | `mise run cluster:lite`                       | k3d cluster + a CNI + lightweight Postgres, Temporal dev server, in-memory SpiceDB (`infra/local/deps.yaml`) |
| Port-forwards | `mise run dev:forward`                      | forwards the deps to localhost (Postgres 5432, Temporal 7233/8233, SpiceDB 50051); leave running     |
| Inner loop    | run the service natively                    | set the env contract and run it in any editor/IDE or `go run ./services/<svc>/cmd/server` — no build/deploy |
| In-cluster    | `mise run service:deploy -- <svc>`          | one-shot build → `k3d image import` → `helm upgrade` (for edge/auth/e2e testing); **no watch loop**  |
| Migrations    | `mise run db:migrate`                       | applies each service's migrations to the local Postgres                                              |
| Teardown      | `mise run cluster:stop` / `cluster:delete`   | stops (keeps image cache) / deletes the cluster                                                      |

**Native, against real dependencies.** The service binary runs on the host; it reaches the k3d-hosted deps through the
`dev:forward` port-forwards and the standard env contract (`DATABASE_URL`, `TEMPORAL_HOST_PORT`, `SPICEDB_ENDPOINT`).
There is nothing to rebuild or redeploy on save — you just re-run. When you genuinely need the service *in* the cluster
(exercising the edge, auth, or e2e), `service:deploy` does a single build-import-upgrade against the production
`infra/helm/service` chart with the `local` values overlay — a one-shot, not a watch loop.

**Lightweight deps in the inner loop only.** `infra/local/deps.yaml` ships throwaway Postgres / Temporal-dev / in-memory
SpiceDB so a service has something to talk to without paying for the full platform. Their production counterparts (CNPG,
the Temporal Helm chart, the SpiceDB chart, the observability stack, the gateway and auth edge) run in the full-platform
local tier (`cluster:full`) and in dev/staging/prod, where their operators and ordering behave correctly
([ADR-0016](0016-environment-parity.md)).

**What is not swapped out, ever:** the Kubernetes API, the service chart, the service images, the env contract (
`DATABASE_URL`, `TEMPORAL_HOST_PORT`, OTLP, SpiceDB), the Postgres major version. A bug reproduced locally reproduces in
staging and prod. `service:deploy` loads images into k3d directly (no registry round-trip).

### Service mesh

No dedicated service mesh (Istio, Linkerd, Consul Connect) is deployed. Sidecar meshes inject a proxy
container per pod — at 100 services that's 100+ extra containers on the hot path, against ADR-0000's per-service cost
principle — and the heavier ones (Istio, Consul Connect) add a CRD surface or a mandatory dependency the team size
cannot absorb.

**Cilium covers CNI + mesh as one component.** Its sidecarless eBPF mode provides mTLS (WireGuard node-to-node
encryption), L7 network policies, and per-flow observability (Hubble) without an injected proxy or a second
component. **Hubble UI is deployed as the cluster's network / service-map dashboard** — live service-to-service flows,
dropped connections, and L7 traffic — and is the audit surface for the NetworkPolicy-based internal trust boundary
([ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md)); it is exposed auth-gated at the `hubble.<host>` subdomain
(its React Router is hardwired to basename `/`, so it must be served at a root origin, not under a path prefix). Cilium is
installed
from day one because CNI cannot be hot-swapped on a live cluster.

### Disaster recovery

Three-node HA tolerates single-node failure with no downtime; etcd quorum survives. A full-cluster loss recovers via
(`terraform apply`, if the project provisions its own infra) → `ansible-playbook bootstrap` → ArgoCD reconciling from
git → CNPG restoring Postgres from PITR. On pre-provided infra the hosts already exist, so recovery starts at the
Ansible
step.
Detection target <2 min (Uptime Kuma); recovery target <30 min; RPO ≈ WAL archive interval. Rehearsed quarterly
alongside the backup restore drill above.

## Consequences

### Positive

- Three-node HA from day one removes the "rebuild to HA later" migration entirely.
- Same k3s API end-to-end; local and prod differ in detail, not in shape.
- Growth triggers tied to measurable conditions, not opinion.
- External S3 for durable storage removes MinIO as a production component.
- Provisioning is reproducible from git — plus one Terraform state when the project owns its infra, or the committed
  Ansible inventory when the infra is pre-provided. Terraform is not a day-one dependency.

### Negative / Risks

- Three compute nodes cost more than one. Accepted; the alternative (later HA migration) is a maintenance window we
  never want to plan.
- k3s on bare metal is more ops than managed K8s. Mitigated by Ansible roles as the codified operational knowledge.
- Cilium is more complex to debug than Flannel (eBPF programs, `cilium status`, Hubble CLI). Mitigated by the Helm
  chart being committed and ArgoCD managing upgrades after the initial bootstrap.
- External bucket fees grow with retention. Mitigated by lifecycle policies (cold-tier after 30 days) — configured in
  Terraform when it owns the bucket, or applied to the pre-provided bucket directly.

### Follow-ups

- **(Per-project, not day one)** `infra/terraform/modules/<provider>/` (e.g. `hetzner`) for compute instances, network,
  LB, DNS, firewall, bucket — added when a project provisions its own infrastructure.
- `infra/ansible/roles/` for `k3s_server`, `cilium`, `hardening`, `unattended_upgrades`, `node_exporter`, plus an
  inventory template for pre-provided hosts.
- `infra/helm/platform/{cilium,traefik,cert-manager,minio}/` with local and prod values.
- `docs/cluster/growth-plan.md` (triggers and responses).
- `docs/cluster/local-vs-prod.md` (parity table, divergences).
- `docs/cluster/dr-runbook.md` (full-cluster recovery).
- Quarterly DR drill as a Temporal `Schedule`.

## Rules

- Production runs on plain compute instances (never managed Kubernetes). When the project provisions its own infra,
  provisioning is Terraform under `infra/terraform/`; Terraform is a per-project tool, not run or deployed by default.
  When infra is pre-provided, Terraform is skipped and Ansible bootstraps the existing hosts from a committed inventory.
- Every environment runs k3s with three control-plane nodes (embedded etcd). Adding workers follows the
  resource-pressure trigger.
- Local development runs on k3d in two tiers ([ADR-0016](0016-environment-parity.md)): a fast inner loop (the service
  run natively against lightweight deps) and a full-platform tier (`cluster:full`) running the real charts at a single
  replica, delivered by ArgoCD — the same deploy mechanism the persistent dev/staging/prod clusters use.
- Ingress is Traefik with TLS via cert-manager. Ory Oathkeeper sits behind Traefik as the edge identity filter
  ([ADR-0009](0009-api-gateway.md)); there is no API-management gateway in the default stack.
- Object storage in production is an external S3-compatible bucket. Non-prod (local full tier, dev, staging) uses
  in-cluster MinIO behind the same S3 API ([ADR-0016](0016-environment-parity.md)).
- Database backups are written off-cluster to the same external bucket and rehearsed quarterly.
- Storage class is k3s `local-path` until the storage-scale trigger fires, then Longhorn.
- CNI is Cilium from day one. k3s is installed with `--flannel-backend=none --disable-network-policy`. Cilium is
  bootstrapped by the Ansible `cilium` role (before ArgoCD) and adopted by ArgoCD for upgrades.
- A new cluster bootstraps with `ansible-playbook bootstrap` → `kubectl apply` of the ArgoCD root Application, preceded
  by `terraform apply` only when the project provisions its own infra. No further manual steps.
- Growth from day-one topology happens only on a documented trigger firing, captured in a new ADR.
- No dedicated service mesh is deployed. Sidecar meshes (Istio, Linkerd, Consul Connect) are ruled out by per-service
  resource cost and component count. Cilium covers CNI + zero-trust + L7 policies + Hubble observability in a single
  component with no per-pod proxy overhead.
- Cilium NetworkPolicy is the internal service-to-service trust boundary; the default is deny and each service declares
  its allowed callers ([ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md)). Hubble UI (auth-gated at `hubble.<host>`)
  is the dashboard and audit surface for cluster network flows.
