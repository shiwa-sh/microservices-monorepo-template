# ADR-0014: Frontend Stack & Conventions

- **Status:** Accepted
- **Date:** 2026-05-20
- **Deciders:** Platform team
- **Related:** [ADR-0001](0001-language-and-runtime.md), [ADR-0002](0002-monorepo.md), [ADR-0008](0008-api-contracts.md), [ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md), [ADR-0011](0011-observability.md)

## Context

The single Next.js app under `apps/frontend/` is the front door for landing pages, the authenticated product panel, an internal admin entry point, and the developer portal — all as route groups ([ADR-0002](0002-monorepo.md)). Earlier ADRs pin language, runtime, deployment, codegen, and auth integration. They do not pin how the app is built day-to-day: rendering model, data fetching shape, styling system, forms, lint, testing, browser-side observability, performance gates.

This ADR is the single entry point for a newcomer working on the frontend.

### Pinned by earlier ADRs (not relitigated)

| Concern                   | Decision                                                                                      | ADR                                                            |
|---------------------------|-----------------------------------------------------------------------------------------------|----------------------------------------------------------------|
| Framework                 | Next.js, single app, route groups `(landing\|panel\|admin\|devportal)`                        | [0001](0001-language-and-runtime.md), [0002](0002-monorepo.md) |
| Language + runtime        | TypeScript, Bun as the only JS runtime (no Node anywhere)                                     | [0001](0001-language-and-runtime.md)                           |
| Workspaces                | Bun workspaces, no Turborepo                                                                  | [0002](0002-monorepo.md)                                       |
| API clients               | Generated via `openapi-typescript` + `openapi-fetch` from per-service `openapi.yaml`          | [0008](0008-api-contracts.md)                                  |
| Login UI                  | Custom Next.js driving Kratos self-service flows in `(landing)/auth/`                         | [0010](0010-auth.md)                                           |
| Developer portal          | Route group, not a separate app                                                               | [0009](0009-api-gateway.md)                                    |
| Container                 | Next.js standalone output, tagged `frontend:<git-sha>`, deployed via the shared service chart | [0002](0002-monorepo.md), [0004](0004-gitops.md)               |
| Cross-route-group imports | Lint-forbidden                                                                                | [0002](0002-monorepo.md)                                       |

This ADR adds the remaining first-party choices.

## Decision drivers

1. **One stack for every route group.** Landing, panel, admin, and devportal share the same primitives.
2. **RSC-first.** Use Server Components and the generated SDK on the server where possible; ship the smallest possible client bundle.
3. **Figma-shaped.** The design system is the contract with design; the codebase reflects Figma tokens, not the other way around.
4. **Per-app cost matters.** Adding a route group or a new SDK consumer must not require a per-consumer toolchain decision.
5. **Boring defaults, strict gates.** Strong defaults make a small team productive; CI catches regressions.

## Considered options

- **Pages Router** — App Router is stable and fits the server-heavy, small-client-bundle goal. Pages Router does not.
- **CSS Modules / CSS-in-JS** — viable, but Untitled UI's design contract is in Tailwind tokens; mixing two systems doubles design-system surface.
- **shadcn/ui as the primitives source** — strong contender; rejected because Untitled UI's Figma compatibility is the load-bearing requirement. Untitled UI tokens *could* layer onto shadcn primitives later if a specific primitive is missing.
- **TanStack Router / Remix / Astro** — Next.js is pinned by [ADR-0001](0001-language-and-runtime.md), [ADR-0002](0002-monorepo.md).
- **Biome + minimal ESLint for `@next/eslint-plugin-next`** — second-best. Rejected to keep one tool; the Next-specific rules we care about are caught by `next build`, by Lighthouse-CI, and by typed `next/image` + `next/font` APIs.
- **Vitest / Jest for unit tests** — Bun ships a Jest-compatible test runner; pulling in a second runner duplicates what the platform's only JS runtime already provides.
- **next-intl day one** — premature without a locale on the roadmap; deferred behind a trigger.
- **Vercel `flags`** — Vercel-runtime-specific; rejected for an in-cluster Bun runtime.
- **Storybook** — useful but not load-bearing for a 3–8 person team with one app where Figma is already the isolated visual catalog. Deferred; component tests cover behaviour and a single in-repo "kitchen sink" page (see below) covers visual sanity. Re-evaluated if a second frontend app lands or a dedicated design-system maintainer joins.

## Decisions

### Rendering: App Router, Server Components default

Components are server components by default; `"use client"` is added only at the smallest necessary interactivity boundary. Server Actions are permitted for form submission against the route group's owning service; cross-service mutations go through the service's REST API and use the workflow-handle pattern from [ADR-0006](0006-temporal.md).

Every route segment ships `loading.tsx` and `error.tsx`. Every route-group root additionally ships `not-found.tsx`. A lint rule enforces presence.

### Data fetching

- **Server components** fetch via the generated SDKs in `libs/ts/sdks/<service>/`. A repo-local `libs/ts/server-fetch/` wraps `openapi-fetch` with a server-only fetcher that forwards the Kratos session cookie and W3C trace context. Direct `fetch` to service URLs is forbidden.
- **Client components** use TanStack Query wrapping the same SDKs. Query keys derive from `operationId`; invalidation rules live next to the queries in `libs/ts/server-fetch/`.
- **Mutations** prefer Server Actions when single-service. Cross-service flows return a `202 Accepted` workflow handle ([ADR-0006](0006-temporal.md)) and the client polls via the workflow-handle helper.

### Styling: Tailwind v4 + Untitled UI tokens

- Tailwind CSS v4 is the styling system. CSS Modules and CSS-in-JS are forbidden in app code; the only exception is third-party components that ship their own styles.
- Design tokens come from Untitled UI's Tailwind preset, committed under `libs/ts/ui/tokens/`. Token edits are PRs; upstream Untitled UI bumps are tracked in `libs/ts/ui/UPSTREAM.md` with a yearly bump cadence.
- Class composition uses `clsx` + `tailwind-merge`, re-exported as `cn()` from `libs/ts/ui/`.
- Dark mode via `next-themes`; the theme is set as `data-theme` on the root element.

### Component library: `libs/ts/ui/`

`libs/ts/ui/` is the only place primitives live (Button, Input, Card, Modal, Toast, Form, Table, etc.). Route groups compose them; they do not duplicate them. The library is built from Untitled UI components ported into the repo as committed source — not runtime-fetched.

Heuristic: if two route groups would copy a component, it belongs in `libs/ts/ui/`.

Icons are `lucide-react` (the icon set Untitled UI uses).

**Kitchen-sink page.** `apps/frontend/src/app/(devportal)/devportal/kitchen-sink/page.tsx` renders every primitive in `libs/ts/ui/` once. It is the cheap alternative to Storybook: one route, no separate toolchain, gated by the (devportal) Kratos session. Every primitive added to `libs/ts/ui/` gets a `<Section>` there in the same PR.

### Forms

- `react-hook-form` for state and validation orchestration.
- `zod` for schemas. Schemas for OpenAPI operations are generated from the spec under `tools/codegen/zod-gen/` and committed at `libs/ts/sdks/<service>/schemas/`. The `mise run gen:zod` task is included in `mise run gen:all` and drift-checked by `ci-drift.yml`.
- A single `<Form>` primitive in `libs/ts/ui/` wires react-hook-form + zod + the design-system inputs. Hand-rolled form wiring is a review-blocker.

### Client state

- URL state for filters, pagination, tab selection: **`nuqs`**.
- RSC + URL state covers most cases. For genuine client-only state outliving a component tree, use **Zustand** stores under `apps/frontend/src/stores/`. Redux and MobX are forbidden.
- React Context is for theming and per-route-group session bootstrapping only — not cross-cutting state.

### Auth wiring

- Next.js middleware checks the Kratos session on every request under `(panel)`, `(admin)`, `(devportal)`. `(landing)` is public except for its `auth/` subtree (Kratos flows).
- The middleware reads the Kratos session cookie and forwards a session-id header to server components via `headers()`. Server components never call Kratos directly.
- The frontend never mints, decodes, or validates JWTs. Service calls from server components attach the user's Kratos cookie; Tyk validates the JWT it issues ([ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md)).

### Lint and format: Biome only

Biome is the single lint+format tool for the frontend.

- Configured in `biome.json` at the repo root with the `recommended` and `correctness` rule sets at error level, plus the strict additions: `noExplicitAny`, `noNonNullAssertion`, `useExhaustiveDependencies`, `useImportType`, `noUnusedImports`, `noUnusedVariables`, `useAwait`, `noFloatingPromises`, `noConsole` (server uses `pino`; client uses the OTel-aware logger from `libs/ts/observability/`).
- `biome ci` runs in `lint.yml`; `biome format --write` is a lefthook pre-commit hook.
- **ESLint is not installed.** Next-specific concerns are caught by `next build` + Lighthouse-CI + typed `next/image` / `next/font` APIs.

### Testing

- **`bun test`** for unit and component tests — Bun's built-in Jest-compatible runner. No Vitest, no Jest: Bun is the only JS runtime ([ADR-0001](0001-language-and-runtime.md)), and shipping a third-party runner duplicates what's already in the toolchain. Component tests use Testing Library with `happy-dom` registered via `bunfig.toml` `preload`.
- **Playwright** for end-to-end tests, one suite per route group under `apps/frontend/e2e/(landing|panel|admin|devportal)/`. Suites run against `mise run dev:up` plus the route group's owning services.
- **MSW** for mocking SDK calls in unit/component tests. MSW is forbidden in e2e: e2e runs against real services.
- Coverage thresholds per route group in `bunfig.toml`; CI fails below threshold.

### Observability

The browser side of [ADR-0011](0011-observability.md) is wired here.

- `@opentelemetry/sdk-trace-web` + `@opentelemetry/instrumentation-fetch` initialised in a client-only entry at `apps/frontend/src/observability/client.ts`. Trace IDs propagate via `traceparent` on outbound fetches, joining the same trace as the upstream services.
- **Grafana Faro** is the browser RUM agent. Web Vitals (LCP, INP, CLS), JS errors, and session traces forward through a Tyk-fronted ingest route to the cluster's OTel Collector gateway, landing in the same Loki/Tempo backends as services.
- Next.js server logs are structured JSON via **`pino`**, stdout-only, enriched with `trace_id` from the active span. `console.log` is Biome-forbidden.
- Build embeds `SERVICE_VERSION` from the git SHA so traces and errors are version-attributable.

### Performance gates

- `@next/bundle-analyzer` runs in CI; per-route-group budgets in `apps/frontend/perf-budget.json` fail the build on regression.
- **Lighthouse-CI** runs on every PR, with **LCP < 2.5 s**, **INP < 200 ms**, **CLS < 0.1** as merge gates on the mobile profile (the harder bar).
- Images go through `next/image`; fonts through `next/font`. `<img>` and `@font-face` are forbidden.

### i18n: deferred behind a trigger

No i18n library is adopted day one. All user-facing strings live as TS constants in `libs/ts/ui/strings/<route-group>.ts` — one file per route group. When the first non-English locale is on the roadmap, an ADR amendment adopts `next-intl` and migrates the strings. The shape (one file per route group) is chosen so the migration is mechanical.

### Feature flags: OpenFeature SDK, noop provider

`@openfeature/web-sdk` is wired with a noop provider day one. Application code calls flags through the OpenFeature API from the start; the concrete backend (GrowthBook, Unleash, Flipt, etc.) is adopted on first gradual-rollout requirement via an ADR amendment. Avoids a rip-and-replace later.

### Build and runtime

- `next build` with `output: "standalone"`. The container runs `bun server.js` (Bun executes the file directly; `bun run` is reserved for package-script names).
- One `apps/frontend/Dockerfile`. Multi-stage: build under `oven/bun:<pinned>`; runtime under the smallest pinned Bun image ([ADR-0001](0001-language-and-runtime.md) — Node.js is not installed).
- Image is `frontend:<git-sha>`, deployed via the shared service chart ([ADR-0004](0004-gitops.md)) with route-group ingress paths declared in `infra/gitops/services/<env>/values/frontend.yaml`.

### Local development

- `mise run -C apps/frontend dev` runs `bun run dev` against `mise run dev:up`. SDK requests target services on `localhost` ports established by the dev-up port-forwards ([ADR-0003](0003-cluster-topology.md)).
- HMR is left to Next.js defaults; no custom dev server.

## Consequences

### Positive

- Newcomers find the entire frontend story in one ADR plus citations.
- Server-first rendering keeps bundles small without sacrificing the design system.
- A single design-token source (Untitled UI) gives Figma↔code parity for free.
- Biome-only is the smallest possible TS toolchain — install, format, and lint in one binary.
- Form, fetch, and state primitives are repo-wide; route groups do not fork them.
- Browser traces continue the same trace ID as upstream services, closing the loop with [ADR-0011](0011-observability.md).

### Negative / Risks

- **Biome lacks Next-specific lints.** Mitigated by `next build` + Lighthouse-CI + typed `next/image` / `next/font` APIs. Not a behavioural gap, a different enforcement surface.
- **Untitled UI ports are committed source.** Upgrading from upstream is a real PR, not a `bun update`. Mitigated by `libs/ts/ui/UPSTREAM.md` and a yearly bump.
- **OpenTelemetry-JS web SDK is heavier than Faro alone.** Accepted; the trace continuity between browser and services is worth the bytes, re-evaluated under the perf gates.
- **Deferring i18n risks a painful retrofit.** Mitigated by keeping all strings in one file per route group from day one.
- **Server Actions are relatively new.** Mitigated by limiting them to single-service mutations; cross-service flows use the well-trodden REST + workflow-handle path.

### Follow-ups

- `apps/frontend/` scaffold with the four route groups, middleware, `loading.tsx` / `error.tsx` baselines.
- `libs/ts/ui/` with Untitled UI ports, Tailwind v4 preset, tokens, and `cn()`.
- `libs/ts/server-fetch/` with the server-only fetcher and TanStack Query glue.
- `tools/codegen/zod-gen/` and the `mise run gen:zod` task; inclusion in `mise run gen:all` and `ci-drift.yml`.
- `biome.json` at repo root with the strict ruleset above.
- `apps/frontend/perf-budget.json` and `apps/frontend/lighthouserc.json`.
- `apps/frontend/Dockerfile` (Bun-only, standalone output).
- `apps/frontend/src/observability/{client,server}.ts` wiring OTel-JS, Faro, and `pino`.
- `infra/gateway/apis/frontend-observability.yaml` ingest route for OTel + Faro from the browser.
- `docs/frontend/conventions.md` short pointer file linking back to this ADR.

## Rules

- The frontend is one Next.js App-Router application at `apps/frontend/`. Pages Router is not used.
- Server Components are the default. `"use client"` is added at the smallest interactivity boundary.
- Server Actions are permitted only for mutations against the route group's owning service. Cross-service mutations call the service's REST API and use the workflow-handle pattern from [ADR-0006](0006-temporal.md).
- Every route segment ships `loading.tsx` and `error.tsx`; every route-group root additionally ships `not-found.tsx`.
- Server components fetch via `libs/ts/server-fetch/`. Client components use TanStack Query wrapping the generated SDKs. Direct `fetch` to service URLs is forbidden.
- Hand-written request/response types are forbidden; only types from `libs/ts/sdks/<service>/` are used. Zod schemas for forms are generated from the OpenAPI spec and committed.
- Tailwind v4 is the styling system. CSS Modules, CSS-in-JS, and inline `<style>` are forbidden in app code.
- Design tokens come from `libs/ts/ui/tokens/`. Tokens are not redefined per route group.
- Primitives live in `libs/ts/ui/`. Route groups compose them; they do not duplicate them.
- A primitive added to `libs/ts/ui/` is added to `apps/frontend/src/app/(devportal)/devportal/kitchen-sink/page.tsx` in the same PR.
- Icons are `lucide-react`. Other icon sets require an ADR amendment.
- Forms use react-hook-form + zod via the `<Form>` primitive in `libs/ts/ui/`. Hand-rolled form wiring is forbidden.
- URL state uses `nuqs`. Client-only state outside a component tree uses Zustand under `apps/frontend/src/stores/`. Redux and MobX are forbidden.
- Next.js middleware enforces the Kratos session on `(panel)`, `(admin)`, `(devportal)`. The frontend never mints, decodes, or validates JWTs.
- Biome is the only lint+format tool, configured with the strict ruleset in `biome.json`. ESLint is not installed.
- `bun test` covers unit/component tests with `happy-dom` preloaded via `bunfig.toml`; Playwright covers e2e per route group. MSW is forbidden in e2e. Vitest and Jest are not used.
- Browser observability is OpenTelemetry-JS + Grafana Faro, exporting through a Tyk-fronted ingest route to the cluster's OTel Collector gateway ([ADR-0011](0011-observability.md)).
- Server-side logs are structured JSON via `pino` to stdout. `console.log` is Biome-forbidden.
- Bundle budgets in `apps/frontend/perf-budget.json` and Lighthouse-CI thresholds (LCP < 2.5 s, INP < 200 ms, CLS < 0.1, mobile profile) are merge gates.
- Images go through `next/image`; fonts through `next/font`. `<img>` and `@font-face` are forbidden.
- No i18n library is adopted; user-facing strings live in `libs/ts/ui/strings/<route-group>.ts`. A locale beyond English requires an ADR amendment adopting `next-intl`.
- Feature flags go through `@openfeature/web-sdk` with a noop provider day one. The concrete backend is adopted via an ADR amendment on first gradual-rollout requirement.
- The container runs `bun server.js` from a Next.js standalone build. Node.js is not installed in the image.
