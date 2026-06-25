// Server-side session/identity helper (ADR-0010, ADR-0017). Product surfaces are
// our code, so they authorize in the app — page-level access to /admin is a
// permission check in the RSC layer, not a bare session-cookie presence check
// (that is what proxy.ts does, which only proves *a* session exists). This reads
// the Kratos session via whoami and exposes the identity's roles + AAL so a
// server component can gate on them.
import { cookies } from "next/headers";

// Kratos public API base. In-cluster the Next server reaches it directly; locally
// (host-run `next dev`) point it at the edge, e.g.
// KRATOS_PUBLIC_URL=https://dev.localtest.me:8443/auth
const KRATOS_PUBLIC_URL =
  process.env.KRATOS_PUBLIC_URL ?? "http://ory-kratos-public.platform.svc.cluster.local";

export type Session = {
  identityId: string;
  email?: string;
  roles: string[];
  aal: string;
};

// whoami returns the current Kratos session, or null if unauthenticated.
export async function whoami(): Promise<Session | null> {
  const cookie = (await cookies()).toString();
  if (!cookie.includes("ory_kratos_session")) {
    return null;
  }
  const res = await fetch(`${KRATOS_PUBLIC_URL}/sessions/whoami`, {
    headers: { cookie, accept: "application/json" },
    cache: "no-store",
  });
  if (!res.ok) {
    return null;
  }
  const data = (await res.json()) as {
    identity?: { id?: string; traits?: { email?: string }; metadata_public?: { roles?: unknown } };
    authenticator_assurance_level?: string;
  };
  const rawRoles = data.identity?.metadata_public?.roles;
  const roles = Array.isArray(rawRoles) ? rawRoles.map(String) : [];
  return {
    identityId: data.identity?.id ?? "",
    email: data.identity?.traits?.email,
    roles,
    aal: data.authenticator_assurance_level ?? "aal0",
  };
}

// hasRole reports whether the session carries the given product role. (Service
// APIs make the authoritative SpiceDB check via libs/go/authz; this gates UI.)
export function hasRole(session: Session | null, role: string): boolean {
  if (session === null) {
    return false;
  }
  return session.roles.includes(role);
}
