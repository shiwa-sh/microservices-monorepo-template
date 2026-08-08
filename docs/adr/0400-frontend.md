# ADR-0400: Frontend Stack & Conventions

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0100](0100-language-and-runtime.md), [ADR-0101](0101-monorepo.md), [ADR-0201](0201-gitops.md), [ADR-0302](0302-temporal.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0306](0306-trust-tiers-and-urls.md), [ADR-0500](0500-observability.md), [ADR-0600](0600-local-development-loop.md), [ADR-0601](0601-testing-strategy.md)

## Context

One Next.js app under `apps/frontend/` is the front door for landing pages, the authenticated product panel, an internal admin entry point, and the developer portal, all as route groups. Earlier ADRs pin language, runtime, deployment, codegen, and auth integration. None pins how the app is built day to day.

This ADR is the single entry point for a newcomer working on the frontend.

### Pinned by earlier ADRs

| Concern | Decision | ADR |
| --- | --- | --- |
| App count and route groups | one app, route groups `(landing\|panel\|devportal)` | [ADR-0101](0101-monorepo.md) |
| Language and runtime | TypeScript, Bun as the only JS runtime | [ADR-0100](0100-language-and-runtime.md) |
| Workspaces | Bun workspaces, no Turborepo | [ADR-0101](0101-monorepo.md) |
| API clients | generated from each service's spec | [ADR-0303](0303-api-contracts-and-lifecycle.md) |
| Login UI | custom Next.js driving Kratos self-service flows | [ADR-0304](0304-identity-and-authorization.md) |
| Container | standalone output, deployed via the shared service chart | [ADR-0101](0101-monorepo.md), [ADR-0201](0201-gitops.md) |
| Cross-route-group imports | lint-forbidden | [ADR-0101](0101-monorepo.md) |

## Decision drivers

1. **One stack for every route group.** Landing, panel, admin, and devportal share the same primitives.
2. **Server-first.** Use Server Components and the generated SDK on the server where possible, shipping the smallest client bundle.
3. **Figma-shaped.** The design system is the contract with design.
4. **Per-consumer cost matters.** Adding a route group must not require a toolchain decision.
5. **Boring defaults, strict gates.** Strong defaults make a small team productive; CI catches regressions.

## Considered options

| Concern | Chosen | Rejected, and why |
| --- | --- | --- |
| Router | **App Router** | Pages Router does not fit the server-heavy, small-bundle goal |
| Styling | **Tailwind v4, CSS-first** | CSS Modules and CSS-in-JS are viable, and the design contract is in Tailwind tokens, so mixing systems doubles the design-system surface |
| Primitives | **Untitled UI** | shadcn/ui is a strong contender, and Untitled UI's Figma compatibility is the load-bearing requirement. Untitled tokens could layer onto shadcn primitives later if a primitive is missing |
| Lint and format | **Biome only** | Biome plus a minimal ESLint for the Next plugin is second-best. One tool wins; the Next-specific rules that matter are caught by `next build`, Lighthouse-CI, and the typed `next/image` and `next/font` APIs |
| Unit test runner | **`bun test`** | Vitest and Jest duplicate a Jest-compatible runner the only JS runtime already ships |
| Spec renderer | **Scalar** | Redoc's request console is paywalled, which negates the same-origin "try it" the URL layout was built for. A docs platform (Fern, Mintlify) is a separate stateful service duplicating the SDK codegen. **Stoplight Elements** is the fallback if Scalar's release churn outweighs its momentum |
| Feature flags | **OpenFeature SDK, noop provider** | Vercel `flags` is runtime-specific and wrong for an in-cluster Bun runtime |
| Component catalogue | **an in-repo kitchen-sink route** | Storybook is useful and not load-bearing with one app where Figma is already the isolated visual catalogue. Re-evaluated if a second app lands or a design-system maintainer joins |
| i18n | **deferred behind a trigger** | `next-intl` on day one is premature without a locale on the roadmap |

**The framework is this ADR's to decide.** [ADR-0100](0100-language-and-runtime.md) picks the language and the runtime and stops there. The comparison against TanStack Start, React Router, Astro, and SvelteKit — including the self-hosting posture given the vendor's governance — belongs in this table.

## Decision

### Rendering

Components are server components by default, and `"use client"` is added at the smallest necessary interactivity boundary. Server Actions are permitted for form submission against the route group's owning service; cross-service mutations go through the service's REST API and the workflow-handle pattern ([ADR-0302](0302-temporal.md)).

Every route segment ships `loading.tsx` and `error.tsx`; every route-group root additionally ships `not-found.tsx`.

### Data fetching

| Context | Mechanism |
| --- | --- |
| Server components | the generated SDKs, through an app-local server-only fetcher marked `import "server-only"` that forwards the Kratos session cookie and W3C trace context. Direct `fetch` to service URLs is not used |
| Client components | TanStack Query wrapping the same SDKs, with query keys derived from `operationId` |
| Mutations | Server Actions when single-service; otherwise a `202 Accepted` workflow handle the client polls |

The `server-fetch` directory has **no barrel**: client code imports the client entry and server code the server entry, so server-only modules never leak into a client bundle.

### Styling

Untitled UI's integration is followed exactly, so a component copied from upstream drops in unmodified.

| Element | Decision |
| --- | --- |
| System | Tailwind v4, wired CSS-first with no config file. CSS Modules and CSS-in-JS are not used in app code; third-party components shipping their own styles are the exception |
| Tokens | Untitled UI's committed token file, imported by the global stylesheet. **There is no JS token mirror** — a TypeScript consumer needing a raw value reads the CSS variable |
| Plugins and variants | the global stylesheet registers the React Aria state-variant and animate plugins and declares Untitled's custom variants |
| Class composition | Untitled's `cx` and `sortCx`. Hand-written `cn()` helpers are not added |
| Dark mode | `next-themes`, writing Untitled's class on the root element |

Token edits are PRs. Upstream bumps are tracked in `apps/frontend/src/components/UPSTREAM.md` on a yearly cadence.

### Code layout: one app, no first-party packages

There is exactly one consumer of the frontend code. Route groups are folders in that app — one bundle, one `node_modules` — not independent build targets.

**A workspace package earns its keep only on a second independent consumer, or for a generated artifact.** Splitting single-consumer code into packages buys nothing and costs real ceremony: per-package dependencies, peer dependencies to keep a single React instance, transpile configuration, and path aliases. Under Bun's isolated linker that ceremony is load-bearing, so a missing entry is a build break rather than a lint nit.

| Code | Location |
| --- | --- |
| UI primitives | `src/components/{base,application,foundations}/` |
| Design tokens | `src/styles/` |
| `cx` / `sortCx` and helpers | `src/utils/` |
| Server and client fetchers | `src/lib/server-fetch/` |
| Browser and server telemetry | `src/lib/observability/` |
| Feature flags | `src/lib/feature-flags.ts` |
| User-facing strings | `src/strings/<route-group>.ts` |
| Generated API SDKs | `libs/ts/sdks/<service>/` — the **only** `libs/ts` members |

### Component library

Primitives live under `src/components/` in Untitled UI's own layout. Route groups compose them by explicit path and do not duplicate them. The heuristic: if two route groups would copy a component, it belongs under `src/components/`.

Untitled UI ships source you own, built on [React Aria Components](https://react-spectrum.adobe.com/react-aria/) for accessibility, vendored as committed source rather than fetched at runtime. Keeping upstream's folder layout and utility names verbatim is deliberate: it makes adding a component or taking a yearly bump a clean diff rather than a rewrite.

**The kitchen-sink page** renders every primitive once. It is the cheap alternative to Storybook: one route, no separate toolchain, gated by the devportal session. Every primitive added under `src/components/` gets a section there in the same PR.

### Developer portal renderer

The devportal route group renders the OpenAPI specs through **Scalar**, embedded as a client island — never a separate service.

| Property | Detail |
| --- | --- |
| Why Scalar | a built-in, free request console. Being same-origin with `/api` ([ADR-0306](0306-trust-tiers-and-urls.md)), "try it" calls the real edge with the caller's session and needs no CORS |
| What it renders | a **pre-filtered projection**, not the raw specs. `gen:openapi-public` merges the service specs and filters on the `x-audience` ladder, so the renderer only ever sees what its audience may see. The strip is real, not a UI hide ([ADR-0303](0303-api-contracts-and-lifecycle.md)) |
| Self-hosted | the package is bundled by the build rather than loaded from a CDN, and default web fonts are disabled, so nothing is fetched at runtime |
| CSP fit | it injects inline styles, covered by `style-src`; self-hosts its fonts, covered by `font-src 'self'`; and its console fetches same-origin, covered by `connect-src 'self'` |
| Visual island | Scalar ships its own theme and the portal does not reuse Untitled UI primitives. Accepted for one route group: matching a spec renderer to the design system is not worth the maintenance, and the console is worth more than pixel parity |

The **public docs portal** is anonymous with no login, the norm for public API documentation, and renders only `public` operations. It ships only when a public API does. Credential management is a separate authenticated surface ([ADR-0306](0306-trust-tiers-and-urls.md)): viewing docs never requires an account, only managing keys does.

### Forms, state, and auth wiring

| Concern | Decision |
| --- | --- |
| Forms | `react-hook-form` for orchestration, `zod` for schemas. Schemas for spec operations are generated and committed, drift-checked in CI. One `<Form>` primitive wires all three; hand-rolled form wiring is a review-blocker |
| URL state | `nuqs` for filters, pagination, tab selection |
| Client-only state | Zustand, for state genuinely outliving a component tree. Redux and MobX are not used |
| React Context | theming and per-route-group session bootstrapping only, never cross-cutting state |
| Session | the Next.js proxy checks the Kratos session on `(panel)` and `(devportal)`; `(landing)` is public except its auth subtree. The proxy forwards a session-id header to server components, which never call Kratos directly |
| Tokens | the frontend never mints, decodes, or validates JWTs. Server-component calls attach the user's cookie, and Oathkeeper validates it at the edge |

### Content Security Policy

CSP is a frontend responsibility because a strong policy needs a per-request nonce.

- **The nonce is generated in the proxy** per request and set on both the request header and the response header, with `script-src 'nonce-<x>' 'strict-dynamic'`, `object-src 'none'`, and `base-uri 'self'`. Next propagates it to its own scripts. Inline scripts are not used.
- **`connect-src` allowlists first-party telemetry.** A missing entry silently breaks browser RUM, so the kitchen-sink page exercises it.
- **Static hardening is duplicated at the edge** as defence in depth: the directives that never vary are set by a Traefik middleware ([ADR-0305](0305-edge-auth-and-traffic-policy.md)) so every route gets them, not only Next pages.

### CSRF

CSRF protection applies to cookie-authenticated state changes. Bearer-token traffic is not browser-attached and is not exposed.

| Layer | Mechanism |
| --- | --- |
| Cookie | `SameSite=Lax`, `Secure`, `HttpOnly` ([ADR-0304](0304-identity-and-authorization.md)), which alone blocks the classic cross-site POST |
| Kratos self-service flows | Kratos's built-in anti-CSRF cookie and token, which the custom login UI must not disable |
| Server Actions | Next's built-in `Origin` and `Host` check with allowed origins pinned in config. Hand-rolled CSRF tokens are not added on top |
| Other cookie-authenticated mutations | rejected at the edge by an Origin allowlist ([ADR-0305](0305-edge-auth-and-traffic-policy.md)) |

### Lint, test, and gates

| Concern | Decision |
| --- | --- |
| Lint and format | **Biome only**, configured at repo root with `recommended` and `correctness` at error level plus strict additions including `noExplicitAny`, `noNonNullAssertion`, `useExhaustiveDependencies`, `noFloatingPromises`, and `noConsole`. ESLint is not installed |
| Unit and component tests | `bun test` with Testing Library and `happy-dom`, coverage thresholds per route group |
| Mocking in tests | MSW, an in-process double scoped to the test runner |
| End-to-end and visual | owned by [ADR-0601](0601-testing-strategy.md), driven by Playwright from the repo-root workspace. MSW and the development API mock are both forbidden there |
| Bundle size | per-route-group budgets fail the build on regression |
| Web vitals | Lighthouse-CI on every PR, gating on **LCP < 2.5s**, **INP < 200ms**, **CLS < 0.1** on the mobile profile |
| Images and fonts | `next/image` and `next/font`. Raw `<img>` and `@font-face` are not used |

### Observability

The browser side of [ADR-0500](0500-observability.md) is wired here.

- OpenTelemetry web tracing and fetch instrumentation initialise from a client-only entry. Trace IDs propagate on outbound fetches, joining the same trace as the upstream services.
- **Grafana Faro** is the browser RUM agent. Web vitals, JS errors, and session traces forward through a Traefik-fronted ingest route on a vendor-neutral path to the collector's Faro receiver, landing in the same backends as services. Locally, where the dev server runs on the host with no edge, a dev-only route handler shims that path.
- Server logs are structured JSON to stdout, enriched with the active trace id. `console.log` is lint-forbidden.
- The build embeds the version so traces and errors are version-attributable ([ADR-0103](0103-release-and-versioning.md)).

### Deferred capabilities

| Capability | Trigger | Seam |
| --- | --- | --- |
| i18n via `next-intl` | the first non-English locale on the roadmap | all strings already live in one file per route group, so the migration is mechanical |
| A concrete feature-flag backend | the first gradual-rollout requirement | application code already calls flags through the OpenFeature API against a noop provider, so only the provider changes |
| Storybook and a hosted visual review UI | the kitchen-sink route stops scaling | both consume the same committed components and baselines |

### Build and local development

The build produces standalone output and the container runs it under Bun, from a multi-stage Dockerfile whose runtime stage installs no Node ([ADR-0100](0100-language-and-runtime.md)). The image deploys through the shared service chart with route-group ingress paths in the environment's values.

Locally, the dev server runs against `cluster:base` and is reached through the edge. Work on authenticated surfaces uses the real Traefik, Kratos, and Oathkeeper with application data served by the API mock ([ADR-0600](0600-local-development-loop.md)). **The app's auth path is identical to production** — there is no development-only session, bypass, or environment-conditional branch in the session path.

## Consequences

### Positive

- The entire frontend story is one ADR plus citations.
- Server-first rendering keeps bundles small without sacrificing the design system.
- A single design-token source gives Figma-to-code parity.
- Biome-only is the smallest possible TypeScript toolchain: install, format, and lint in one binary.
- Form, fetch, and state primitives are app-wide, so route groups do not fork them.
- Browser traces continue the same trace id as upstream services.

### Negative / Risks

- **The framework choice has no recorded comparison**, which is the largest undocumented commitment in the repo. It is the one gap in this ADR's *Considered options*, and it is tracked rather than papered over.
- **Biome lacks Next-specific lints.** Mitigated by `next build`, Lighthouse-CI, and the typed asset APIs. A different enforcement surface, not a behavioural gap.
- **Untitled UI source is vendored and committed**, so upgrading is a real PR. Mitigated by keeping the upstream layout verbatim, tracking bumps, and taking them yearly.
- **The OpenTelemetry web SDK is heavier than Faro alone.** Accepted; browser-to-service trace continuity is worth the bytes, and the perf gates keep it honest.
- **Deferring i18n risks a painful retrofit.** Mitigated by the one-file-per-route-group string layout.
- **Server Actions are relatively new.** Mitigated by limiting them to single-service mutations.

## Rules

- The frontend is one Next.js App-Router application. Pages Router is not used. `(review-only)`
- Server Components are the default, and `"use client"` is added at the smallest interactivity boundary. `(review-only)`
- Server Actions are permitted only for mutations against the route group's owning service. Cross-service mutations use the REST API and the workflow-handle pattern. `(review-only)`
- Every route segment ships `loading.tsx` and `error.tsx`; every route-group root also ships `not-found.tsx`. `(CI: ci-lint)`
- First-party frontend code lives in the app. A `libs/ts/*` package is created only for a second consumer or a generated artifact. `(review-only)`
- Server components fetch through the server-only fetcher; client components use TanStack Query over the generated SDKs. Direct `fetch` to service URLs is not used. `(CI: ci-lint)`
- Hand-written request and response types are not declared; only generated types are used, and form schemas are generated from the spec. `(CI: ci-drift)`
- Tailwind v4 is the styling system, wired CSS-first. CSS Modules, CSS-in-JS, and inline `<style>` are not used in app code. `(CI: ci-lint)`
- Design tokens come from the committed Untitled UI token file. There is no JS token mirror, and tokens are not redefined per route group. `(review-only)`
- Class composition uses `cx` and `sortCx`. Hand-written helpers are not added. `(CI: ci-lint)`
- Primitives are the vendored Untitled UI source, composed by explicit path and never duplicated. `(review-only)`
- A primitive added under `src/components/` is added to the kitchen-sink page in the same PR. `(review-only)`
- Icons come from the Untitled UI icon set. Another set requires an ADR amendment. `(review-only)`
- Forms use react-hook-form and zod through the shared `<Form>` primitive. `(review-only)`
- URL state uses `nuqs`; client-only state uses Zustand. Redux and MobX are not used. `(CI: ci-lint)`
- The proxy enforces the Kratos session on the authenticated route groups, and the frontend never mints, decodes, or validates JWTs. `(CI: lint:auth-inline)`
- CSP is set in the proxy with a per-request nonce; inline scripts are not used and `connect-src` allowlists the telemetry ingest origin. `(review-only)`
- CSRF rests on the SameSite cookie, Kratos's built-in protection, and the Server Actions Origin check. Hand-rolled CSRF tokens are not added. `(review-only)`
- Biome is the only lint and format tool. ESLint is not installed. `(CI: ci-lint)`
- `bun test` covers unit and component tests. Vitest and Jest are not used. `(review-only)`
- The frontend contains no development-only authentication code. `(CI: lint:auth-inline)`
- Browser observability is OpenTelemetry web plus Faro, exporting through the edge to the collector. `(review-only)`
- Server logs are structured JSON to stdout. `(CI: ci-lint)`
- Bundle budgets and the Lighthouse thresholds are merge gates. `(CI: ci-build)`
- Images go through `next/image` and fonts through `next/font`. `(CI: ci-lint)`
- No i18n library is adopted; strings live in one file per route group. `(review-only)`
- Feature flags go through the OpenFeature API with a noop provider. `(review-only)`
- The container runs the standalone build under Bun, and installs no Node. `(CI: lint:node-scope)`
