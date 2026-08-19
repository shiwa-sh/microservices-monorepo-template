-- migrate:up
-- The read-only inspector role (ADR-0401).
--
-- ADR-0401 states that pgweb and psql are enforced read-only AT THE DATABASE-ROLE
-- LEVEL rather than by convention, and that the user literally cannot UPDATE.
-- pgweb's own `--readonly` flag is one layer; this is the other, and it is the one
-- that holds when someone reaches the same database with psql instead.
--
-- Grants live in each service's migrations because each service owns its schema:
-- the role is cluster-global and created once, but WHAT it may read is a per-database
-- fact that changes whenever a table does. `postInitApplicationSQL` cannot carry
-- them — it runs in one database at bootstrap, and these tables arrive later.
--
-- `ALTER DEFAULT PRIVILEGES` is the half that keeps working: without it the next
-- migration to add a table makes it invisible to the inspector, and the failure is
-- an empty panel rather than an error.
do $$
begin
  if exists (select from pg_roles where rolname = 'readonly') then
    execute 'grant usage on schema public to readonly';
    execute 'grant select on all tables in schema public to readonly';
    execute 'alter default privileges in schema public grant select on tables to readonly';
  end if;
end
$$;

-- migrate:down
do $$
begin
  if exists (select from pg_roles where rolname = 'readonly') then
    execute 'alter default privileges in schema public revoke select on tables from readonly';
    execute 'revoke select on all tables in schema public from readonly';
    execute 'revoke usage on schema public from readonly';
  end if;
end
$$;
