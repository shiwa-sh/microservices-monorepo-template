# payment

A workflow-backed service (ADR-0006). `POST /charges` is idempotent on
`Idempotency-Key` and returns a workflow handle. The `Charge` workflow:

1. Records the charge as `pending`
2. Runs `SettleActivity` (the PSP integration, mocked here)
3. Marks the charge `settled` — or `failed` if `SettleActivity` errors

Demonstrates idempotency, compensation, and the cross-service workflow handle shape (ADR-0006).
