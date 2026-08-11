# Postgres major-version upgrade

The logical-replication cutover, step by step. The decision is [ADR-0300](../adr/0300-data.md); this is the procedure.

Run it once per Postgres release cadence, in a planned maintenance window. Every service takes a brief **read-only** window at cutover; nothing is offline.

## Why not in-place

CNPG supports an in-place major upgrade, which rewrites the existing cluster and has no cheap rollback: if the new major misbehaves, the old data directory is already converted. Logical replication keeps the old cluster running and serving until the moment traffic moves, so **rollback is repointing the connection back**.

The cost is that both clusters run at once, so the node set needs headroom for two copies of the data during the window.

## Before the window

1. **Read the release notes for the target major**, specifically the incompatibilities section. Logical replication does not carry sequences or large objects automatically, and it will not warn you about a behaviour change.
2. **Check extension availability** at the target major for every extension in use ([ADR-0300](../adr/0300-data.md)). An extension that has not been packaged for the new major blocks the upgrade.
3. **Verify a restore.** The quarterly drill ([ADR-0207](../adr/0207-cluster-storage.md)) is the safety net if the cutover goes wrong. Do not run this procedure on a cluster whose last restore drill failed.
4. **Confirm every table has a primary key.** Logical replication of `UPDATE` and `DELETE` needs a replica identity; a table without one silently fails to replicate those rows.

## The cutover

```sh
# 1. Create the target cluster at the new major, in the same namespace.
#    Size it as the current one; it will hold a full copy.
kubectl apply -f infra/helm/platform/postgres/  # with the new Cluster manifest

# 2. Wait for it to report healthy and empty.
kubectl -n platform get cluster postgres-<newmajor> -w

# 3. Start replication from the current cluster and wait for initial sync.
#    Watch lag until it is steady near zero.
kubectl -n platform exec -it postgres-<newmajor>-1 -- \
  psql -c "SELECT * FROM pg_stat_subscription;"

# 4. Quiesce writes. Scale the service Deployments to zero, or put the edge
#    into maintenance. This is the start of the read-only window.

# 5. Confirm zero lag, then advance sequences on the target — logical
#    replication does not carry them.

# 6. Repoint the services: update the connection secret to the new cluster
#    and let the operator materialise it (ADR-0202).

# 7. Scale the services back up. The read-only window ends here.

# 8. Verify: run the smoke suite, then watch error rate and latency.
mise run e2e:smoke
```

## Rolling back

Until step 6, rolling back is deleting the target cluster; nothing has moved.

After step 6 and within the window, repoint the connection secret at the old cluster and scale back up. Writes that landed on the new cluster after cutover are lost, which is why the verification in step 8 happens immediately rather than the next morning.

## After the window

- Keep the old cluster for at least one full backup retention period before deleting it.
- Confirm the first `ScheduledBackup` against the new cluster completes, and that a PITR restore from it works. **A backup that has never been restored is not a backup** ([ADR-0200](../adr/0200-cluster-topology.md)).
- Update the pinned major in the chart values so a fresh environment builds at the new version.
