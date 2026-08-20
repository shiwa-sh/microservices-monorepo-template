// East-west fetcher for cluster-audience services (ADR-0303, ADR-0700).
//
// The sibling `server.ts` dials the EDGE under `/api`, which is right for every
// service a browser could also reach. This one dials a Service by DNS, which is
// the only way to reach a service whose `x-audience` is `cluster` — analytics and
// the authz relation check have no `/api` route at all, by decision: an edge route
// to the event store would be an unauthenticated way to fill it.
//
// The difference that matters is what travels. `server.ts` forwards the user's
// session cookie so the edge can authenticate them; this forwards NOTHING about
// the user, because the caller here is the Next pod acting on its own behalf. A
// page that needs the user's identity resolves it first (lib/auth/session.ts) and
// passes it as data — never as ambient credentials.
import "server-only";

import createClient, { type Client } from "openapi-fetch";

/**
 * A client for one internal service, addressed by its `<SERVICE>_URL` variable.
 *
 * The same convention services use to reach each other, so there is one answer to
 * "how do I call a sibling" rather than one per caller. A missing variable is a
 * thrown error rather than a default: every candidate default is a wrong guess,
 * and a wrong guess fails later, at the first request, as a DNS error nobody
 * connects to a missing configuration value.
 */
export function createInternalClient<Paths extends object>(envVar: string): Client<Paths> {
  const baseUrl = process.env[envVar];
  if (!baseUrl) {
    throw new Error(`${envVar} is not set — an internal service cannot be reached without it`);
  }
  return createClient<Paths>({ baseUrl });
}
