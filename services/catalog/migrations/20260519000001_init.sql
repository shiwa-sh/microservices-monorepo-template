-- migrate:up
create table products (
  id          uuid primary key default gen_random_uuid(),
  name        text not null,
  price_cents integer not null check (price_cents >= 0),
  created_at  timestamptz not null default now()
);

-- migrate:down
drop table products;
