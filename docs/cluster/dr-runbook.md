# Disaster recovery runbook

Recovering from full cluster loss. The targets and the backup design are [ADR-0200](../adr/0200-cluster-topology.md).

## Detection

**Nothing pages anyone.** Alertmanager routes a firing alert to email or to the webhook receiver by its `severity` ([ADR-0502](../adr/0502-alerting-and-on-call.md)), and no escalation service is attached to that webhook. An overnight incident is found in the morning. Attaching a paging service is the gap to close before an on-call rotation exists.

## Recovery

```sh
# 1. Provision a new node set — ONLY if the project provisions its own infra.
#    On pre-provided infrastructure the hosts already exist; skip to step 2.
cd infra/terraform/environments/<env>
terraform apply

# 2. Apply the Talos machine configs and bootstrap the cluster, plus the
#    in-cluster age key (against the Terraform-produced nodes, or the
#    committed inventory of pre-provided Talos nodes).
cd ../../../talos
terraform apply

# 3. Re-install ArgoCD's root Application; ArgoCD reconciles everything else.
kubectl apply -f infra/gitops/bootstrap/root-application.yaml

# 4. CNPG restores Postgres from PITR in the external bucket. Wait for the
#    `Cluster` CR to report `Phase: Cluster in healthy state`.
kubectl -n platform get cluster postgres -w
```

Rehearsed quarterly against a staging rebuild, tracked as a Temporal `Schedule`.
