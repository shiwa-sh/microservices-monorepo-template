# frontend

The single Next.js application (ADR-0002 §"TypeScript workspaces"). Route
groups separate audiences inside one deploy unit:

```text
src/app/(landing)/    — public marketing, sign-in
src/app/(panel)/      — authenticated customer panel
src/app/(admin)/      — staff-only tile that links to /internal/admin (Lowdefy)
src/app/(devportal)/  — third-party API docs
```

`(landing)/auth/{login,register,recovery,settings}` render the Kratos
self-service flows (ADR-0010) through the shared `components/auth/KratosFlow`
component. They ship in every environment and must stay in sync with the
`selfservice.flows.*.ui_url`s in the Ory chart values
(`infra/helm/platform/ory/values.yaml`).

Route groups must not import from one another (ADR-0002 lint rule).
Generated TS clients live under `libs/ts/sdks/<service>/` and are imported as
`@sdks/<service>` (see `tsconfig.json` paths).
