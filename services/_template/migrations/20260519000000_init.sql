-- migrate:up
create table items (
  id   uuid primary key default gen_random_uuid(),
  name text not null,
  created_at timestamptz not null default now()
);

-- migrate:down
drop table items;
