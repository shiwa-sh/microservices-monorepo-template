# Disaster recovery runbook

Recovering from full cluster loss. The recovery objectives and the backup design are [ADR-0200](../adr/0200-cluster-topology.md); this is the procedure that meets them.

**Your RTO clock starts here, not at the failure.** ADR-0200 commits to under 30 minutes from the start of recovery, and RPO to the WAL archive interval. How long it took anyone to notice is a separate figure, and it is in [`reference/detection-latency.md`](../reference/detection-latency.md).

## Detection

**Nothing pages anyone.** Alertmanager routes a firing alert to email or to the webhook receiver by its `severity` ([ADR-0502](../adr/0502-alerting-and-on-call.md)), and no escalation service is attached to that webhook. This is a decided position, not an oversight: out of hours, expect to find an overnight incident in the morning, and plan the recovery window accordingly. [ADR-0502](../adr/0502-alerting-and-on-call.md) states the trigger that buys this down.

## Recovery

Run every command from the repository root. Step 1 applies only where the project provisions its own infrastructure; on pre-provided hosts, start at step 2.

```sh
# 1. Provision a new node set — instances, network, LB, DNS, firewall, bucket.
terraform -chdir=infra/terraform/environments/<env> apply

# 2. Apply the Talos machine configs and bootstrap the cluster, plus the
#    in-cluster age key. Runs against the Terraform-produced nodes, or the
#    committed inventory of pre-provided Talos nodes.
terraform -chdir=infra/talos/<env> apply

# 3. Re-install Argo CD's root Application; Argo CD reconciles everything else.
kubectl apply -f infra/gitops/bootstrap/root-application.yaml

# 4. CNPG restores Postgres from PITR in the external bucket. Wait for the
#    `Cluster` CR to report `Phase: Cluster in healthy state`.
kubectl -n platform get cluster postgres -w
```

Steps 2 and 4 dominate the wall clock, and step 3's reconciliation overlaps step 4.

Rehearsed quarterly against a staging rebuild, tracked as a Temporal `Schedule`. **Record the elapsed time of each rehearsal**: the objectives in ADR-0200 are measurements, and a rehearsal that does not time itself leaves them as intentions.
