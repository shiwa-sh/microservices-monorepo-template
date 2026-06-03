# Cluster growth plan (ADR-0003)

Triggers, signals, and responses. Each trigger fires a new ADR before action.

| Trigger           | Signal                                                  | Response                                            |
|-------------------|---------------------------------------------------------|-----------------------------------------------------|
| Resource pressure | CPU or memory >70% sustained 7 days across the node set | Add k3s **agent** nodes; control plane stays at 3   |
| Storage scale     | Any PVC >50% of node disk                               | Adopt **Longhorn** as default storage class         |
| Compliance        | Workload with isolation/regulatory requirement          | Provision a **dedicated cluster** for that workload |
