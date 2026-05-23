# Local vs. prod parity (ADR-0003)

| Layer          | Local (k3d)                  | Prod (k3s on Hetzner)             | Same?              |
|----------------|------------------------------|------------------------------------|---------------------|
| Kubernetes API | k3s                          | k3s                                | yes                 |
| Helm charts    | `infra/helm/`                | `infra/helm/`                      | yes                 |
| Service code   | identical image              | identical image                    | yes                 |
| Ingress        | Traefik                      | Traefik                            | yes                 |
| TLS issuer     | mkcert local CA              | Let's Encrypt DNS-01               | no                  |
| LB driver      | klipper-lb                   | hcloud-ccm                         | no                  |
| Object storage | MinIO                        | external S3-compatible bucket      | interface yes       |
| GitOps         | not used                     | ArgoCD                             | no, by design       |
| Sizing         | tiny                         | sized for traffic                  | no                  |

What is NOT swapped out, ever: the Kubernetes API, the chart structure, the service images, the OTLP endpoint shape, the Postgres major version.
