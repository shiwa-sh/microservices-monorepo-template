-- migrate:up
-- A retried checkout must not place a second order (ADR-0003). The client sends an
-- `Idempotency-Key` and the column carries it, so the uniqueness is the database's
-- to enforce rather than a race between two handlers reading before writing — the
-- same shape payment has had since it was written.
--
-- Nullable, because rows written before this existed have no key and the constraint
-- has to tolerate them; a UNIQUE index ignores NULLs, so every real key is still
-- unique. New orders always carry one: the handler requires the header.
alter table orders add column idempotency_key text;
create unique index orders_idempotency_key_idx on orders (idempotency_key);

comment on column orders.idempotency_key is 'pii:none';

-- migrate:down
drop index orders_idempotency_key_idx;
alter table orders drop column idempotency_key;
