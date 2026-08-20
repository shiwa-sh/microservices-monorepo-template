// The authoritative page gate (ADR-0700, ADR-0304).
//
// ADR-0700 requires the analytics route group to be "page-gated by `Checker`
// rather than by a bare session check", and states why in one line: the proxy
// proves only that a session EXISTS. Anyone signed in has one. Whether this
// particular person may read funnels is a relationship in OpenFGA, and asking is
// the difference between an authenticated page and an authorised one.
//
// `Checker` is a Go library, and this render layer is not Go — so the question
// goes to the authz service's east-west relation check, which reaches the same
// OpenFGA the services do.
import "server-only";

import { forbidden, unauthorized } from "next/navigation";
import { getSession } from "@/lib/auth/session";
import { createInternalClient } from "@/lib/server-fetch/internal";

type AuthzPaths = {
  "/authorize/relation": {
    post: {
      requestBody: {
        content: { "application/json": { subject: string; relation: string; object: string } };
      };
      responses: { 200: { content: { "application/json": { allowed: boolean } } } };
    };
  };
};

/**
 * Requires the caller to hold `relation` on `object`, or renders the matching
 * access-denial page.
 *
 * The two denials are different facts and get different pages (ADR-0400): no
 * session is `unauthorized()`, because signing in is the remedy; a session that
 * lacks the grant is `forbidden()`, rendered at the denied address, because
 * signing in again changes nothing and the address is what the user gives to
 * whoever can grant access.
 */
export async function requireRelation(relation: string, object: string): Promise<void> {
  const session = await getSession();
  if (session === null) {
    unauthorized();
  }

  const authz = createInternalClient<AuthzPaths>("AUTHZ_URL");
  const { data, error } = await authz.POST("/authorize/relation", {
    body: { subject: `user:${session.identityId}`, relation, object },
  });
  // An error here is authz being unreachable, which is not a denial. Throwing
  // sends it to `error.tsx`, so "we could not ask" never renders as "you may not"
  // — the two need opposite responses from the reader.
  if (error) {
    throw new Error(`authz relation check failed for ${object}`);
  }
  if (!data.allowed) {
    forbidden();
  }
}
