-- name: CreateOrg :one
insert into orgs (name) values ($1) returning id, name;

-- name: GetOrg :one
select id, name from orgs where id = $1;

-- name: AddMember :exec
insert into org_members (org_id, user_id, role) values ($1, $2, $3)
on conflict do nothing;
