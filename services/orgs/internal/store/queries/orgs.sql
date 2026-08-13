-- name: ListOrgs :many
select
  id,
  name
from orgs
order by created_at desc limit 100;

-- name: CreateOrg :one
insert into orgs (id, name) values ($1, $2) returning id, name;

-- name: GetOrg :one
select
  id,
  name
from orgs
where id = $1;

-- name: UpdateOrg :one
update orgs set name = $2
where id = $1
returning id, name;

-- name: DeleteOrg :exec
delete from orgs where id = $1;

-- name: AddMember :exec
insert into org_members (org_id, user_id, role) values ($1, $2, $3)
on conflict do nothing;
