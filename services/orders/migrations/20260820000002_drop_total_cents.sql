-- migrate:up
-- The CONTRACT half of the expand/contract that introduced `orders.total`
-- (ADR-0300). The expand migration added the money columns and backfilled from
-- `total_cents`; this removes the column now that nothing reads it.
--
-- It is a SEPARATE migration on purpose, and the separation is the whole point. A
-- column dropped in the same release that stopped writing it leaves any instance
-- still running the previous image writing to a column that no longer exists —
-- which is an outage during a rolling deploy, not a schema tidy. Two migrations in
-- two releases is what puts a deploy boundary between the two.
--
-- `lint:money` deliberately tolerated this column until now; after this migration
-- a reappearance is a finding rather than a leftover.
alter table orders drop column total_cents;

-- migrate:down
-- The rollback restores the column and rebuilds it from the money column, so a
-- revert lands on a schema the previous image can write to. Not null with a
-- default of 0 rather than a plain add: the old code writes it on every insert,
-- and a nullable column would let a row through that the old code cannot read.
alter table orders add column total_cents integer not null default 0;
update orders set total_cents = (total::numeric * 100)::integer;
