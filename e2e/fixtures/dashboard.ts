// Shared "Gated" stage helpers (ADR-0017 edge authz). Each ops dashboard is
// checked with request contexts (no browser): unauthenticated is denied, a bare
// AAL1 product session is forbidden, and only the AAL2 operator holding the
// per-tool grant passes Oathkeeper. Accept:*/* takes Oathkeeper's json error
// branch (401/403) rather than the html login redirect.
import { type APIRequestContext, expect, request } from "@playwright/test";
import { OPERATOR_STATE, USER_STATE, opsURL } from "./env";

async function statusFor(tool: string, storageState?: string): Promise<number> {
  const ctx: APIRequestContext = await request.newContext({
    ignoreHTTPSErrors: true,
    storageState,
  });
  try {
    const res = await ctx.get(`${opsURL(tool)}/`, {
      maxRedirects: 0,
      headers: { accept: "*/*" },
    });
    return res.status();
  } finally {
    await ctx.dispose();
  }
}

export async function expectUnauthenticatedDenied(tool: string): Promise<void> {
  expect([401, 302, 303]).toContain(await statusFor(tool));
}

export async function expectAal1Forbidden(tool: string): Promise<void> {
  expect([403, 302, 303]).toContain(await statusFor(tool, USER_STATE));
}

export async function expectOperatorAllowed(tool: string): Promise<void> {
  expect(await statusFor(tool, OPERATOR_STATE)).toBe(200);
}

// The operator is an operator everywhere but only holds specific tools; a tool
// with no grant is forbidden even for them (the fine gate, ADR-0010).
export async function expectOperatorForbidden(tool: string): Promise<void> {
  expect([403, 302, 303]).toContain(await statusFor(tool, OPERATOR_STATE));
}
