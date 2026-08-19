// Browser-side fetcher (ADR-0400). Pairs with TanStack Query in the app's
// providers. Calls target the gateway through Traefik on the same origin.
"use client";

import createClient, { type Client } from "openapi-fetch";
import { denialMiddleware } from "@/lib/auth/denial";

// Flat API (ADR-0306): every service is reached under the shared /api prefix on
// the same origin. The typed resource path (e.g. /products) selects the endpoint;
// which service serves it is hidden behind the edge, so no service name is passed.
export function createBrowserClient<Paths extends object>(): Client<Paths> {
  // Same denial handling as the server client (lib/auth/denial.ts), so a session
  // that expires mid-session is not a browser-only special case. On this side the
  // interrupt is thrown out of the fetch, which means it lands wherever the call
  // was made from: inside TanStack Query it is caught and must be re-thrown during
  // render to reach its boundary — the panel providers do that once, for every
  // query and mutation in the group.
  const client = createClient<Paths>({ baseUrl: "/api" });
  client.use(denialMiddleware);
  return client;
}
