-- migrate:up
create table orders (
  id          uuid primary key default gen_random_uuid(),
  product_id  uuid not null,
  quantity    integer not null check (quantity > 0),
  total_cents integer not null check (total_cents >= 0),
  status      text not null check (status in ('pending','confirmed','failed')),
  created_at  timestamptz not null default now()
);

-- migrate:down
drop table orders;
