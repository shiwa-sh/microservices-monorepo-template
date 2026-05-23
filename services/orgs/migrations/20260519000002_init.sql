-- migrate:up
create table orgs (
  id   uuid primary key default gen_random_uuid(),
  name text not null,
  created_at timestamptz not null default now()
);

create table org_members (
  org_id  uuid not null references orgs(id) on delete cascade,
  user_id text not null,
  role    text not null check (role in ('admin','member')),
  primary key (org_id, user_id)
);

-- migrate:down
drop table org_members;
drop table orgs;
