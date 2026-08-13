// Next.js config (ADR-0400). Standalone output is required for the Bun-only
// Dockerfile. One first-party package lives outside the app — `@libs/id`, the
// identifier codec both languages share (ADR-0003) — and it ships as TypeScript
// source with no build step, so Next has to transpile it like app code.

// Server Actions CSRF allowlist (ADR-0305, ADR-0400), derived from the one edge
// origin the BROWSER uses (EDGE_PUBLIC_ORIGIN) — deliberately not the internal
// origin a pod dials, which differs by port in-cluster and would allowlist the
// wrong host. See src/lib/server-fetch/server.ts for the split. Next compares the list against `new URL(origin).host`, which
// INCLUDES the port — hence `.host`, not the bare hostname: a `dev.localtest.me`
// entry can never match an `https://dev.localtest.me:8443` origin.
//
// This list is only consulted when Origin ≠ (X-Forwarded-)Host. ADR-0306 puts `/`
// and `/api` on the same origin, so that is already the common case and this is
// defence in depth. Note it is BUILD-time: `output: "standalone"` freezes this
// config into server.js, so a deployed image only carries an entry if the build
// passed EDGE_PUBLIC_ORIGIN — which pins that image to one env host and breaks the
// build-once/promote-by-digest flow in ADR-0103. Leave it unset in CI builds.
const edgeOrigin = process.env.EDGE_PUBLIC_ORIGIN;
const allowedOrigins = edgeOrigin ? [new URL(edgeOrigin).host] : [];

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  // A workspace package of TypeScript source, not a built dependency.
  transpilePackages: ["@libs/id"],
  reactStrictMode: true,
  typedRoutes: true,
  // `cluster:full` serves the host-run `next dev` through the edge at
  // dev.localtest.me:8443 — a different origin than localhost — so allow it,
  // otherwise Next blocks the cross-origin dev/HMR requests. Dev-server-only;
  // the prod build ignores it.
  allowedDevOrigins: ["dev.localtest.me"],
  experimental: {
    serverActions: {
      allowedOrigins,
    },
  },
  env: {
    NEXT_PUBLIC_SERVICE_VERSION: process.env.SERVICE_VERSION ?? "dev",
    NEXT_PUBLIC_DEPLOY_ENV: process.env.DEPLOY_ENV ?? "dev",
  },
  // Stamp the shipped build's identity on every response (ADR-0103), so "did prod
  // actually update?" is answerable from response headers / devtools, and a stale
  // browser bundle is caught by comparing this against the backend's X-App-Version.
  headers() {
    return Promise.resolve([
      {
        source: "/:path*",
        headers: [{ key: "X-App-Version", value: process.env.SERVICE_VERSION ?? "dev" }],
      },
    ]);
  },
};

export default nextConfig;
