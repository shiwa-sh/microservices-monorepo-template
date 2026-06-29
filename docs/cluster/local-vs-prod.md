# Local vs. prod parity (ADR-0003, ADR-0016)

Local runs in **two tiers** ([ADR-0016](../adr/0016-environment-parity.md)):

- **Inner loop** (`mise run cluster:up` + `dev:forward` + run the service **natively**) — k3d plus lightweight dependency stand-ins; the service under change runs on the host. Optimised for iteration speed; parity is at the **interface** (same wire contracts), not the implementation.
- **Full platform** (`mise run cluster:full`) — k3d running the **same charts production runs**, at a single replica. Parity is at the **implementation**: CNPG, the Temporal chart, SpiceDB, in-cluster MinIO, the observability stack, and the edge + auth. This is the end-to-end / pre-merge tier, and the same configuration CI and per-PR previews use.

The table below compares the **full-platform tier** with production — the inner-loop differences are called out in the last column.

| Layer          | Local full tier (k3d)                                      | Prod (k3s on compute)               | Same?                         |
|----------------|------------------------------------------------------------|-------------------------------------|-------------------------------|
| Kubernetes API | k3s                                                        | k3s                                 | yes                           |
| Helm charts    | `infra/helm/`                                              | `infra/helm/`                       | yes                           |
| Service code   | identical image                                            | identical image                     | yes                           |
| Data tier      | CNPG / Temporal / SpiceDB charts                           | same charts, HA                     | yes (scale differs)           |
| Object storage | in-cluster MinIO                                           | external S3-compatible bucket       | interface yes (S3 API)        |
| Secrets        | SOPS + committed local age key                             | SOPS + per-cluster age key          | yes (mechanism); plaintext no |
| Observability  | `infra/helm/platform/observability`                        | same chart, HA                      | yes (scale differs)           |
| Ingress        | Traefik (k3s-bundled)                                      | Traefik                             | yes                           |
| GitOps         | ArgoCD app-of-apps from committed `master` (`infra/gitops/bootstrap-local/`) | ArgoCD            | full tier yes; inner loop no  |
| TLS issuer     | cert-manager + self-signed ClusterIssuer                   | cert-manager + Let's Encrypt DNS-01 | mechanism yes; issuer no      |
| LB driver      | klipper-lb                                                 | provider cloud-controller-manager   | no                            |
| Sizing         | single replica                                             | sized for traffic                   | no                            |

**Inner loop differences (interface parity only):** the inner loop swaps the data-tier *implementations* for speed — a plain Postgres pod, `temporal server start-dev`, in-memory SpiceDB (`infra/local/deps.yaml`) — and skips MinIO, observability, and the auth edge (bring up the full tier for those). The wire contracts are identical, so a bug reproduced there reproduces in prod; the full tier exists to validate the *implementations* and their wiring.

**What is NOT swapped out, ever (any tier):** the Kubernetes API, the chart structure, the service images, the env contract (`DATABASE_URL`, `TEMPORAL_HOST_PORT`, OTLP endpoint, SpiceDB), and the Postgres major version.

**GitOps locally:** the full tier *is* ArgoCD — `cluster:full` syncs committed `master` through the same app-of-apps prod uses (`infra/gitops/bootstrap-local/`), so sync waves and app discovery are exercised by default. To test uncommitted GitOps wiring, push a branch and point the local root-app at it — see [gitops-local.md](gitops-local.md).
