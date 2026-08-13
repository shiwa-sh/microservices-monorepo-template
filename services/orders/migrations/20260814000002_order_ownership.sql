-- migrate:up
-- Every protected resource belongs to an org, and a user acts only through a role
-- in one (ADR-0304). An order recorded neither, so `order#read: owner or write
-- from org` in model.fga had nothing to resolve against and no handler could ask.
--
-- Authorisation itself reads the OpenFGA tuple, never these columns. They exist so
-- the tuples can be rebuilt from the system of record, and so a collection scoped
-- to one buyer has something to filter on.
alter table orders add column owner_id text;
alter table orders add column org_id uuid;

-- NOT VALID enforces the constraint on every insert and update while leaving rows
-- written before ownership existed alone. Those rows carry no tuple either, so
-- they are readable by an operator and by nobody else — which is the correct
-- answer for an order whose buyer was never recorded.
alter table orders add constraint orders_owner_id_present check (owner_id is not null) not valid;
alter table orders add constraint orders_org_id_present check (org_id is not null) not valid;

comment on column orders.owner_id is 'pii:identifier';
comment on column orders.org_id is 'pii:none';

-- migrate:down
alter table orders drop constraint orders_org_id_present;
alter table orders drop constraint orders_owner_id_present;
alter table orders drop column org_id;
alter table orders drop column owner_id;
