// Resolving the session cookie to an identity, server-side (ADR-0400, ADR-0304).
//
// A page route carries no injected identity header, and that is deliberate rather
// than an omission: Oathkeeper's rules are written per API resource, so a page
// matches none of them and gets a 404, and a catch-all rule would make two rules
// match and Oathkeeper answer 500 — which `lint:api-wildcard` exists to prevent.
// The shared service chart's ingressroute template says so where the middleware is
// omitted.
//
// So the page asks Kratos directly. The proxy has already established that a
// session cookie EXISTS before the request reaches a protected route; this
// establishes whose it is, which is a different question and the only one an
// authorization check can be built on.
import "server-only";

import { headers } from "next/headers";

// The in-cluster Kratos public API. East-west by Service DNS rather than through
// the edge: the edge would route this back to Oathkeeper, which is the component
// asking the question in the first place.
const KRATOS_PUBLIC =
  process.env.KRATOS_PUBLIC_URL ?? "http://ory-kratos-public.platform.svc.cluster.local";

export type Session = {
  /** The Kratos identity id. This is the `user:<id>` subject in every authz check. */
  identityId: string;
  /** The session's assurance level — `aal1` or `aal2` (ADR-0304). */
  aal: string;
};

type WhoamiResponse = {
  identity?: { id?: string };
  authenticator_assurance_level?: string;
};

/**
 * The current session, or null when there is none.
 *
 * Null is a legitimate answer and not an error: an expired cookie is the ordinary
 * case, and the caller decides whether that means `unauthorized()` or a public
 * render. A transport failure throws, because "Kratos is unreachable" and "you are
 * not signed in" must not look the same to a page deciding what to show.
 */
export async function getSession(): Promise<Session | null> {
  const cookie = (await headers()).get("cookie");
  if (!cookie) {
    return null;
  }
  const res = await fetch(`${KRATOS_PUBLIC}/sessions/whoami`, {
    headers: { cookie },
    // A session check is never cached: the whole point is that it reflects the
    // request being served, and Next caches server fetches by default.
    cache: "no-store",
  });
  if (res.status === 401 || res.status === 403) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`kratos whoami: ${res.status}`);
  }
  const body = (await res.json()) as WhoamiResponse;
  const identityId = body.identity?.id;
  if (!identityId) {
    return null;
  }
  return {
    identityId,
    aal: body.authenticator_assurance_level ?? "aal1",
  };
}
