-- name: ListItems :many
select
  id,
  name,
  created_at
from items
order by created_at desc limit 100;

-- name: CreateItem :one
insert into items (id, name) values ($1, $2) returning id, name, created_at;
