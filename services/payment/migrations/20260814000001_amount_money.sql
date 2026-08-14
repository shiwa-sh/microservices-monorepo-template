-- migrate:up
-- The charge amount joins the shared money type (ADR-0300, ADR-0003), for the same
-- reasons as catalog's price and orders' total.
alter table charges add column amount numeric(19, 4);
alter table charges add column currency text;

update charges set amount = amount_cents::numeric / 100, currency = 'EUR' where amount is null;

alter table charges alter column amount set not null;
alter table charges alter column currency set not null;
alter table charges add constraint charges_amount_positive check (amount > 0);
alter table charges add constraint charges_currency_iso4217 check (currency ~ '^[A-Z]{3}$');

comment on column charges.amount is 'pii:none';
comment on column charges.currency is 'pii:none';

-- amount_cents stays until nothing reads it.

-- migrate:down
alter table charges drop constraint charges_currency_iso4217;
alter table charges drop constraint charges_amount_positive;
alter table charges drop column currency;
alter table charges drop column amount;
