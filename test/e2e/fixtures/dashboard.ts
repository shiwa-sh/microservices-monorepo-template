// Shared "Gated" stage helpers (ADR-0306 edge authz). Each ops dashboard is
// checked with request contexts (no browser): unauthenticated is denied, a bare
// AAL1 product session is forbidden, and only the AAL2 operator holding the
// per-tool grant passes Oathkeeper. Accept:*/* takes Oathkeeper's json error
// branch (401/403) rather than the html login redirect.
import { type APIRequestContext, expect, request } from "@playwright/test";
import { OPERATOR_STATE, USER_STATE, opsURL } from "./env";

// The local resolver occasionally answers a *.ops.<host> lookup with a transient
// EAI_AGAIN under the load of a full suite run (many contexts + port-forwards),
// surfacing as a spurious gated-check failure rather than a real authz result. A
// DNS/connection blip is not the signal these tests exist to catch, so retry the
// bare request a few times; a genuine 401/403/200 returns immediately.
async function statusFor(
  tool: string,
  storageState?: string,
): Promise<{ status: number; location: string }> {
  const ctx: APIRequestContext = await request.newContext({
    ignoreHTTPSErrors: true,
    storageState,
  });
  try {
    let lastErr: unknown;
    for (let attempt = 0; attempt < 4; attempt++) {
      try {
        const res = await ctx.get(`${opsURL(tool)}/`, {
          maxRedirects: 0,
          headers: { accept: "*/*" },
        });
        return { status: res.status(), location: res.headers().location ?? "" };
      } catch (err) {
        // Only retry transient name-resolution / connection errors, never a real
        // HTTP response (get() resolves for any status, so those never throw).
        if (!/EAI_AGAIN|ENOTFOUND|ECONNREFUSED|ECONNRESET/.test(String(err))) {
          throw err;
        }
        lastErr = err;
        await new Promise((r) => setTimeout(r, 500 * (attempt + 1)));
      }
    }
    throw lastErr;
  } finally {
    await ctx.dispose();
  }
}

export async function expectUnauthenticatedDenied(tool: string): Promise<void> {
  expect([401, 302, 303]).toContain((await statusFor(tool)).status);
}

export async function expectAal1Forbidden(tool: string): Promise<void> {
  expect([403, 302, 303]).toContain((await statusFor(tool, USER_STATE)).status);
}

// The operator passes when the answer came from the TOOL rather than from the gate
// in front of it. A 200 is that answer, and so is a redirect the tool sends to its
// own origin — a tool that carries its own login sends one, and the shell it lands
// on is what the sibling render test asserts. Reading only the status would call
// that a failure, having just proved the grant worked.
//
// It cannot be confused with a denial. Oathkeeper answers this Accept with 401/403,
// and its html branch redirects to the product apex's /auth/login — a different
// origin, which is exactly what this checks.
export async function expectOperatorAllowed(tool: string): Promise<void> {
  const { status, location } = await statusFor(tool, OPERATOR_STATE);
  if (status === 200) {
    return;
  }
  // Resolved against the tool's origin, because a tool's own redirect is usually
  // path-only ("/login") while the edge's is absolute and elsewhere.
  const target = location ? new URL(location, `${opsURL(tool)}/`).href : "";
  expect(
    [301, 302, 303, 307, 308].includes(status) && target.startsWith(`${opsURL(tool)}/`),
    `operator was not passed through to ${tool}: ${status} → ${target || "(no location)"}`,
  ).toBe(true);
}
