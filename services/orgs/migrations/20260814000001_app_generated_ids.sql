-- migrate:up
-- The service mints the primary key, not the column (ADR-0003). Two things follow
-- from the default being gone: the identifier exists BEFORE the insert, so a
-- handler can log it, tag its span, and report it on a write that never lands; and
-- the value is a UUIDv7 from `libs/go/id` rather than the UUIDv4 `gen_random_uuid`
-- returns, so the key carries a time prefix and the index appends instead of
-- scattering. A default of any kind would be a second generator with neither
-- property, reached by exactly the inserts that forgot to pass an id.
alter table orgs alter column id drop default;

-- migrate:down
alter table orgs alter column id set default gen_random_uuid();
