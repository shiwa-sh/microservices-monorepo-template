-- migrate:up
create table charges (
  id              uuid primary key default gen_random_uuid(),
  order_id        uuid not null,
  amount_cents    integer not null check (amount_cents > 0),
  status          text not null check (status in ('pending','settled','failed','refunded')),
  idempotency_key text not null unique,
  created_at      timestamptz not null default now()
);

-- migrate:down
drop table charges;
