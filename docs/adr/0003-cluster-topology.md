# ADR-0003: Cluster Topology & Hosting

- **Status:** Accepted
- **Date:** 2026-07-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0002](0002-monorepo.md), [ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md), [ADR-0015](0015-naming-and-identifiers.md)

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
  ├── <host>/api/*      ─▶ Oathkeeper (identity) ─▶ backend service (per ADR-0009)
  ├── <host>/(landing|panel|devportal)/* ─▶ Next.js frontend pod (one app, route groups per ADR-0014)
  ├── lowdefy.ops.<host>/ ─▶ Oathkeeper (operator + AAL2) ─▶ Lowdefy pod (internal admin, per ADR-0012)
  ├── grafana.ops.<host>/ ─▶ Oathkeeper (operator + AAL2) ─▶ Grafana
  └── hubble.ops.<host>/  ─▶ Oathkeeper (operator + AAL2) ─▶ Hubble UI (service map / network flows, ADR-0025)
```

**Traefik is the only ingress; Oathkeeper is an auth filter behind it, not a second gateway.** Traefik does TLS,
hostname/path routing, load balancing, and rate limiting; Ory Oathkeeper validates identity and injects identity headers
([ADR-0009](0009-api-gateway.md)). There is no API-management gateway in the default stack.

**DNS:**

- One wildcard `*.<env>.example.com` `A` record per environment, pointing at the LB IP.
- `cert-manager` requests one wildcard certificate per environment via DNS-01 against the project's DNS provider.
- `external-dns` is not used. The wildcard absorbs new services.

**Cluster networking (CNI): Cilium + WireGuard + default-deny, day one.** Cilium is the CNI; k3s is installed with
`--flannel-backend=none --disable-network-policy`, and Cilium is installed by the Ansible bootstrap role before ArgoCD is
started, then adopted by ArgoCD for upgrades. CNI cannot be hot-swapped on a live cluster, so the security posture is set
at bootstrap, not retrofitted. Three postures are on from day one and are checked invariants, not aspirations:

1. **Default-deny segmentation.** The `platform-baseline` CiliumNetworkPolicy (`infra/helm/platform/network-policies/`)
   sets `enableDefaultDeny` for both ingress and egress across every platform pod; all allows are additive. Each backend
   grants its own callers in the sibling templates, and each service's chart declares which callers may reach it. Because
   internal calls carry forwarded identity headers and no token ([ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md)),
   NetworkPolicy is what guarantees only sanctioned callers reach a service's port.
2. **Encryption in transit.** WireGuard transparent encryption (`encryption.type: wireguard`) encrypts all east-west pod
   traffic node-to-node — a Cilium config flag, not a component. Multi-tenant PII and audit posture do not ship
   plaintext east-west, and there is no separate mesh to run for it.
3. **Egress control, including the cloud metadata endpoint.** Default-deny egress means no pod reaches the internet
   unless a policy grants it (only Loki/Tempo, to object storage on :443 in prod). A clusterwide policy additionally
   denies the link-local cloud metadata address `169.254.169.254/32` outright, so even a future broad egress grant
   cannot become an SSRF path to instance credentials.

Hubble (bundled) provides per-flow visibility — via the `hubble` CLI, the drop metrics scraped into Grafana, and the
Hubble UI service map auth-gated at `hubble.ops.<host>` ([ADR-0025](0025-service-map-apm-ui.md)) — and is the audit
surface for these policies. Application observability (SLO/RED/logs/traces/profiling) is Grafana's
([ADR-0011](0011-observability.md)/[ADR-0025](0025-service-map-apm-ui.md)).

### Storage

**Day one:**

- **Block storage:** k3s `local-path` provisioner. PVCs are node-local NVMe directories.
- **Object storage in production:** external S3-compatible bucket, per environment — created by Terraform when the
  project provisions its own infra, or referenced by configuration when the bucket is pre-provided. Loki,
  Tempo, CNPG backups (and Pyroscope where profiling is enabled, [ADR-0011](0011-observability.md)) all write here;
  Prometheus keeps a local TSDB and needs no bucket (the Mimir Scale swap does — [ADR-0011](0011-observability.md)).
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

`mise run cluster:base` creates the k3d cluster and the local floor; a service's own tasks add what it declares, and the inner loop is then **native
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
| Cluster+floor | `mise run cluster:base`                       | k3d cluster + a CNI + Traefik, cert-manager, Postgres, Kratos, Oathkeeper. Temporal and in-memory OpenFGA are opt-in `dep:*` components (`infra/local/deps.yaml`) |
| Port-forwards | `mise run dev:forward`                      | forwards the deps to localhost (Postgres 5432, Temporal 7233/8233, OpenFGA 8080); leave running     |
| Inner loop    | run the service natively                    | set the env contract and run it in any editor/IDE or `go run ./services/<svc>/cmd/server` — no build/deploy |
| In-cluster    | `mise run service:deploy -- <svc>`          | one-shot build → `k3d image import` → `helm upgrade` (for edge/auth/e2e testing); **no watch loop**  |
| Migrations    | `mise run db:migrate`                       | applies each service's migrations to the local Postgres                                              |
| Teardown      | `mise run cluster:stop` / `cluster:delete`   | stops (keeps image cache) / deletes the cluster                                                      |

**Native, against real dependencies.** The service binary runs on the host; it reaches the k3d-hosted deps through the
`dev:forward` port-forwards and the standard env contract (`DATABASE_URL`, `TEMPORAL_HOST_PORT`, `OPENFGA_API_URL`).
There is nothing to rebuild or redeploy on save — you just re-run. When you genuinely need the service *in* the cluster
(exercising the edge, auth, or e2e), `service:deploy` does a single build-import-upgrade against the production
`infra/helm/service` chart with the `local` values overlay — a one-shot, not a watch loop.

**Lightweight deps in the inner loop only.** `infra/local/deps.yaml` ships throwaway Postgres / Temporal-dev / in-memory
OpenFGA so a service has something to talk to without paying for the full platform. Their production counterparts (CNPG,
the Temporal Helm chart, the OpenFGA chart, the observability stack, the gateway and auth edge) run in the full-platform
local tier (`cluster:full`) and in dev/staging/prod, where their operators and ordering behave correctly
([ADR-0016](0016-environment-parity.md)).

**What is not swapped out, ever:** the Kubernetes API, the service chart, the service images, the env contract (
`DATABASE_URL`, `TEMPORAL_HOST_PORT`, OTLP, OpenFGA), the Postgres major version. A bug reproduced locally reproduces in
staging and prod. `service:deploy` loads images into k3d directly (no registry round-trip).

### Service mesh — and why Cilium, not Linkerd

No dedicated service mesh (Istio, Linkerd, Consul Connect) is deployed. A category note first: **Linkerd is not a CNI
and not a Cilium substitute** — it rides *on top of* a CNI via per-pod sidecars. The real day-one choice is a CNI-level
one, compared on **security capability** (dashboards and RAM aside):

| Security capability                              | flannel only        | flannel + Linkerd            | **Cilium + WireGuard**            |
|--------------------------------------------------|---------------------|------------------------------|-----------------------------------|
| L3/L4 default-deny segmentation                  | none (flat)         | meshed app traffic only      | all pods                          |
| Data-tier protection (PG/MinIO/OpenFGA)          | wide open           | only if the data tier is meshed (fiddly) | NetworkPolicy         |
| Cryptographic workload identity (anti-spoof)     | —                   | mTLS certs (meshed)          | label identity; SPIFFE optional   |
| Encryption in transit (east-west)                | plaintext           | meshed only                  | all pods (WireGuard)              |
| L7 authz (route/method)                          | —                   | fine-grained                 | Envoy, coarse — already have Oathkeeper + OpenFGA |
| Egress control + DNS/FQDN (exfil/C2/metadata SSRF) | —                 | not Linkerd's job            | FQDN + L3 egress                  |

flannel *silently ignores* applied NetworkPolicy objects — worse than "no policy," because it grants false confidence;
so flannel-only is no east-west security at all, indefensible for a multi-tenant platform. Between the two real options,
Cilium wins on **breadth**: the controls it adds (data-tier segmentation, egress/metadata-SSRF control) map onto the
highest-frequency, highest-impact cluster attacks — lateral movement to Postgres, metadata-endpoint credential theft
(the Capital One shape). The controls Linkerd would add over Cilium (cert identity, fine L7) are depth *behind* those,
and the L7 layer is already covered by Oathkeeper at the edge and OpenFGA in-app. Linkerd would also leak the data tier
unless CNPG/MinIO/OpenFGA are meshed — the fiddly sidecar-vs-Job work our Job-heavy bootstrap makes painful. Cilium
mutual-auth / SPIFFE can add cert identity later with no sidecars, if wanted.

**Cilium covers CNI + mesh as one component.** Its sidecarless eBPF mode provides transparent WireGuard encryption,
L7 network policies, and per-flow observability (Hubble) without an injected proxy or a second component. A sidecar mesh
would add 100+ proxy containers on the hot path at 100 services, against ADR-0000's per-service cost principle. **Hubble flows are the audit surface** for the NetworkPolicy-based internal trust boundary
([ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md)) — live service-to-service flows, dropped connections, and L7
traffic, available via the `hubble` CLI, the drop metrics scraped into Grafana, and the bundled **Hubble UI** service map,
auth-gated at `hubble.ops.<host>` ([ADR-0025](0025-service-map-apm-ui.md) — including the accepted workload-name-collapse
trade-off). Application observability is Grafana's ([ADR-0011](0011-observability.md)). Cilium is installed
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
- **Default-deny is enforced**, not assumed: the `platform-baseline` CiliumNetworkPolicy sets `enableDefaultDeny` for
  ingress and egress across every platform pod; all allows are additive.
- **WireGuard transparent encryption is on** for all east-west pod traffic (`encryption.type: wireguard` in the Cilium
  values). Plaintext east-west is not shipped.
- **Egress blocks the cloud metadata endpoint.** Default-deny egress already denies unrequested internet egress; a
  clusterwide policy additionally denies `169.254.169.254/32` so no egress grant can become a metadata-SSRF path to
  instance credentials.
- A new cluster bootstraps with `ansible-playbook bootstrap` → `kubectl apply` of the ArgoCD root Application, preceded
  by `terraform apply` only when the project provisions its own infra. No further manual steps.
- Growth from day-one topology happens only on a documented trigger firing, captured in a new ADR.
- No dedicated service mesh is deployed. Sidecar meshes (Istio, Linkerd, Consul Connect) are ruled out by per-service
  resource cost and component count. Cilium covers CNI + zero-trust + L7 policies + Hubble observability in a single
  component with no per-pod proxy overhead.
- Cilium NetworkPolicy is the internal service-to-service trust boundary; the default is deny and each service declares
  its allowed callers ([ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md)). Hubble flows (via the `hubble` CLI,
  Grafana drop metrics, and the Hubble UI at `hubble.ops.<host>` — [ADR-0025](0025-service-map-apm-ui.md)) are the audit
  surface for cluster network flows; application observability is Grafana's ([ADR-0011](0011-observability.md)).
