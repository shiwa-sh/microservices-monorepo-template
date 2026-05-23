-- name: ListItems :many
select id, name, created_at from items order by created_at desc limit 100;

-- name: CreateItem :one
insert into items (name) values ($1) returning id, name, created_at;
