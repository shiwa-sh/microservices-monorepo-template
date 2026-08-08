# orders

The checkout saga (ADR-0302). The `Checkout` workflow lives here because orders owns the *business process*, even though it calls `catalog` for prices and `payment` to charge — process-owner rule.

`POST /orders` returns a 202 with a workflow handle (the `WorkflowHandle` schema in `openapi.yaml`). Status is observable via `GET /orders/{id}`.
