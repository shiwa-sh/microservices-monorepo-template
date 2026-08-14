-- name: ListProducts :many
select
  id,
  name,
  price,
  currency
from products
order by created_at desc limit 100;

-- name: GetProduct :one
select
  id,
  name,
  price,
  currency
from products
where id = $1;

-- The write still fills price_cents alongside price: the column is not dropped yet
-- (see the migration), it is NOT NULL, and a row inserted without it would fail.
-- The follow-up migration that drops it removes this too.
-- name: CreateProduct :one
insert into products (id, name, price, currency, price_cents)
values ($1, $2, $3, $4, ($3::numeric * 100)::integer)
returning id, name, price, currency;

-- name: UpdateProduct :one
update products set name = $2, price = $3, currency = $4, price_cents = ($3::numeric * 100)::integer
where id = $1
returning id, name, price, currency;

-- name: DeleteProduct :exec
delete from products where id = $1;
