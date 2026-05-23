-- name: ListProducts :many
select id, name, price_cents from products order by created_at desc limit 100;

-- name: GetProduct :one
select id, name, price_cents from products where id = $1;

-- name: CreateProduct :one
insert into products (name, price_cents) values ($1, $2) returning id, name, price_cents;
