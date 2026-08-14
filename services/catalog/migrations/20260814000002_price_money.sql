-- migrate:up
-- A monetary amount is `numeric` in the database and a string on the wire, never a
-- float and never an integer of minor units (ADR-0300, ADR-0003). `price_cents`
-- was the last of those: a defensible representation in the abstract, and not the
-- one this platform chose.
--
-- numeric(19,4): 4 fractional digits covers every ISO 4217 minor unit (including
-- the 3-digit ones and the 4-digit UYW), and 19 total digits is what fits a
-- 64-bit-safe range without reaching for numeric's unbounded form, which no index
-- likes. Currency travels with the amount, because an amount without one is not a
-- quantity of anything.
alter table products add column price numeric(19, 4);
alter table products add column currency text;

-- Backfill from the integer minor units, then make both columns the contract. The
-- currency is EUR because that is what every committed example and fixture in this
-- repo has always meant by `_cents`; there was never a second one.
update products set price = price_cents::numeric / 100, currency = 'EUR' where price is null;

alter table products alter column price set not null;
alter table products alter column currency set not null;
alter table products add constraint products_price_nonnegative check (price >= 0);
alter table products add constraint products_currency_iso4217 check (currency ~ '^[A-Z]{3}$');

comment on column products.price is 'pii:none';
comment on column products.currency is 'pii:none';

-- price_cents stays for now. Dropping a column and rewriting every reader in one
-- migration is how a deploy window becomes an outage: the old code is still running
-- while the new schema lands. It goes in a follow-up once nothing reads it.

-- migrate:down
alter table products drop constraint products_currency_iso4217;
alter table products drop constraint products_price_nonnegative;
alter table products drop column currency;
alter table products drop column price;
