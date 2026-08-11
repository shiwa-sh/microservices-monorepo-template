# Database migrations

How-to for writing and applying schema migrations. The decision (dbmate, sqlc, CNPG, per-service databases) is [ADR-0300](../adr/0300-data.md); this is the operational procedure.

## Write a migration

1. Create the file under `services/<service>/migrations/`, timestamped, in dbmate format with `-- migrate:up` and `-- migrate:down` sections.
2. **Write the `down` section for yourself, not for production.** It is what lets you re-run the migration locally. A production rollback is a forward fix — a new migration that reverses the change ([ADR-0300](../adr/0300-data.md)) — so never plan on running it there.
3. Regenerate the typed data layer after any schema or query change: `mise run gen:sqlc`. Commit what it writes; CI drift-checks it against your SQL.
4. Run `mise run lint:sql` before you push. It rejects a `timestamp` where `timestamptz` belongs, and an untagged column holding personal data ([ADR-0301](../adr/0301-data-lifecycle-privacy.md)).

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

CNPG `ScheduledBackup` + WAL archiving to the off-cluster bucket is the recovery path; restore is rehearsed quarterly ([ADR-0207](../adr/0207-cluster-storage.md), [disaster-recovery](disaster-recovery.md)).
