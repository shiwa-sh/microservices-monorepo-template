# Database migrations

How-to for writing and applying schema migrations. The decision (dbmate, sqlc, CNPG, per-service databases) is [ADR-0300](../adr/0300-data.md); this is the operational procedure.

## Write a migration

- Migrations live under `services/<service>/migrations/` as timestamped SQL files (dbmate format, `-- migrate:up` / `-- migrate:down`).
- A `down` section exists for local development. **Down migrations are not used in production**: a production rollback is a forward fix, a new migration that reverses the change ([ADR-0300](../adr/0300-data.md)).
- After changing schema or queries, regenerate the typed data layer: `mise run gen:sqlc` (sqlc). Generated code is committed and drift-checked in CI ([ADR-0300](../adr/0300-data.md)).
- SQL is linted by sqruff: `mise run lint:sql`.

## Apply migrations locally

```sh
mise run db:migrate            # applies each service's migrations to the local Postgres
```

The inner loop runs against the throwaway local Postgres; the full tier and deployed environments run CNPG ([ADR-0205](../adr/0205-environment-parity.md)).

## Apply migrations in a deployed environment

Migrations run as an **init container** before the service container, never by hand. An advisory lock prevents concurrent runs across replicas. A schema change ships with the service image that depends on it, so the ordering is deploy-time rather than a separate run.

A change that could break running code follows expand → migrate → contract, with at least one deploy boundary between expand and the code switch ([ADR-0300](../adr/0300-data.md)).

## Authz-relevant migrations

A migration that adds or changes an authz-relevant table must land together with the OpenFGA schema and the dual-write path ([ADR-0304](../adr/0304-identity-and-authorization.md)). Never mutate authz-relevant rows outside the workflow dual-write.

## Backups & recovery

CNPG `ScheduledBackup` + WAL archiving to the off-cluster bucket is the recovery path; restore is rehearsed quarterly ([ADR-0200](../adr/0200-cluster-topology.md), [disaster-recovery](disaster-recovery.md)).
