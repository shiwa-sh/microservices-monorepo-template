// Browser-side fetcher (ADR-0400). Pairs with TanStack Query in the app's
// providers. Calls target the gateway through Traefik on the same origin.
"use client";

import createClient, { type Client } from "openapi-fetch";

// Flat API (ADR-0306): every service is reached under the shared /api prefix on
// the same origin. The typed resource path (e.g. /products) selects the endpoint;
// which service serves it is hidden behind the edge, so no service name is passed.
export function createBrowserClient<Paths extends object>(): Client<Paths> {
  return createClient<Paths>({ baseUrl: "/api" });
}
