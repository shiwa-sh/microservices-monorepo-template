// The one place an access denial from the API becomes UI (ADR-0400, ADR-0304).
//
// The split this encodes is the whole design, and it is not a style preference:
//
//   401 — no usable session. Signing in IS the remedy, so the user is sent to the
//         login flow and returned to where they were. A redirect is correct here
//         because it is the fix.
//   403 — a session, and no permission. Signing in again changes nothing, so the
//         user must NOT be redirected: the denied URL is the only useful fact in
//         the situation (it is what they paste to whoever grants access), and a
//         redirect throws it away. Next renders forbidden.tsx in place, 403, same
//         address, back button intact.
//
// Both are raised from the fetch middleware in ../server-fetch/*, so a route or a
// component gets this behaviour by calling the API — no per-call auth branch, and
// nothing for a reviewer to remember to ask for.
//
// Where the decision comes from: the services do. Oathkeeper authenticates /api/*
// at the edge and the service makes the authoritative OpenFGA check (ADR-0304), so
// the frontend never computes a permission — it renders the one it was handed. It
// could not compute one anyway: nothing here can reach Kratos (the frontend's
// egress does not admit it, ADR-0400), which is why there is no whoami() helper.
import { forbidden, unauthorized } from "next/navigation";

// Next tags its access-fallback throws with this digest. It is an internal shape,
// named here once so the coupling is visible and pinned to the Next version in
// package.json — see the authInterrupts note in next.config.mjs.
const ACCESS_FALLBACK_DIGEST = "NEXT_HTTP_ERROR_FALLBACK";

// Raises the matching interrupt for an API status, and returns for anything else.
// Throws — the caller does not branch on a return value.
export function raiseForAuthDenial(status: number): void {
  if (status === 401) {
    unauthorized();
  }
  if (status === 403) {
    forbidden();
  }
}

// Is this thrown value one of the interrupts above? Needed on the browser side:
// TanStack Query catches everything a queryFn throws, and an interrupt only
// reaches its boundary if it is re-thrown during render — see the panel providers.
export function isAuthDenial(error: unknown): boolean {
  if (typeof error !== "object" || error === null || !("digest" in error)) {
    return false;
  }
  const { digest } = error as { digest: unknown };
  if (typeof digest !== "string") {
    return false;
  }
  const [prefix, status] = digest.split(";");
  return prefix === ACCESS_FALLBACK_DIGEST && (status === "401" || status === "403");
}

// Shared by both fetch clients. Kept here rather than duplicated per client so the
// server and the browser cannot drift on which statuses count.
export const denialMiddleware = {
  onResponse({ response }: { response: Response }) {
    raiseForAuthDenial(response.status);
  },
};
