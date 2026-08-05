// Server-only fetcher (ADR-0014). Wraps openapi-fetch so server components
// call services with the Kratos session cookie and W3C trace context attached.
// Client components must use ./client.ts.
import "server-only";

import { headers } from "next/headers";
import createClient, { type Client } from "openapi-fetch";

// Where THIS PROCESS dials the edge. Two different answers depending on where the
// process runs, which is why the old single EDGE_ORIGIN could not work in-cluster:
//
//   host `next dev`  → https://dev.localtest.me:8443   (k3d maps 8443 → 443)
//   in-cluster pod   → https://dev.localtest.me        (Traefik's 443 directly)
//
// Both must still carry the env HOST: the /api IngressRoutes match on
// Host(<env host>), Oathkeeper injects identity there (ADR-0009/0010), and the
// wildcard cert is issued for it. So a call that skips the edge is unrouted,
// unauthenticated AND untrusted. In-cluster that host resolves to Traefik via the
// CoreDNS rewrite in infra/local/coredns-rewrite.yaml — without it the name
// resolves to the pod's own loopback.
//
// EDGE_PUBLIC_ORIGIN is the browser-facing origin and is a SEPARATE variable
// (see next.config.mjs, which derives the server-action CSRF allowlist from it);
// the two differ by port in-cluster, so one variable cannot serve both.
//
// Falls back to the public origin so a host `next dev` — where the two genuinely
// are the same — needs only one variable set.
//
// No default beyond that: every candidate is a wrong guess (a cluster Service DNS
// name would 404 on the Host match; localhost is a different app). A misconfigured
// environment should say so, not fail as a DNS error at the first server component.
// It cannot come from the request's Host header either — that is client-controlled,
// and this fetcher forwards the user's session cookie.
const API_BASE = process.env.EDGE_INTERNAL_ORIGIN ?? process.env.EDGE_PUBLIC_ORIGIN;

// Flat API (ADR-0017): server components call the edge under the shared /api
// prefix; the typed resource path selects the endpoint, service topology hidden.
// The prefix cannot be relative — a server-side fetch has no document to resolve
// against, so the origin is required even though the browser calls the same paths
// same-origin (see ./client.ts).
export async function createServerClient<Paths extends object>(): Promise<Client<Paths>> {
  if (!API_BASE) {
    throw new Error(
      "No edge origin: server components fetch through the edge and need its origin. " +
        "Set EDGE_PUBLIC_ORIGIN (e.g. https://dev.localtest.me:8443), and " +
        "EDGE_INTERNAL_ORIGIN too when running in-cluster (e.g. https://dev.localtest.me). " +
        "Locally, start the dev server with `mise run dev:frontend`; in a deployed env " +
        "set them in infra/gitops/services/<env>/values/frontend.yaml.",
    );
  }

  const h = await headers();
  const cookie = h.get("cookie") ?? "";
  const traceparent = h.get("traceparent") ?? "";

  return createClient<Paths>({
    baseUrl: `${API_BASE}/api`,
    headers: {
      ...(cookie && { cookie }),
      ...(traceparent && { traceparent }),
    },
  });
}
