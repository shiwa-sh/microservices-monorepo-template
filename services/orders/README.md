# orders

The checkout saga (ADR-0006). The `Checkout` workflow lives here because orders
owns the *business process*, even though it calls `catalog` for prices and
`payment` to charge — process-owner rule.

`POST /orders` returns a 202 with a workflow handle (`api/shared/workflow-handle.yaml`).
Status is observable via `GET /orders/{id}`.
