// Browser-side fetcher (ADR-0014). Pairs with TanStack Query in the app's
// providers. Calls target the gateway through Traefik on the same origin.
"use client";

import createClient, { type Client } from "openapi-fetch";

export function createBrowserClient<Paths extends object>(service: string): Client<Paths> {
  return createClient<Paths>({ baseUrl: `/api/${service}` });
}
