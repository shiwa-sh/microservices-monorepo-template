# catalog

The simplest shop service. Pure CRUD over a single `products` table — no workflows.

Demonstrates: OpenAPI spec → handlers → sqlc queries → dbmate migrations → observability middleware.

```sh
cd services/catalog
mise run migrate    # apply migrations to $DATABASE_URL
mise run run        # http://localhost:8080
```
