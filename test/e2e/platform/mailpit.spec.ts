// Mailpit viewer operator dashboard (ADR-0307, ADR-0306) — non-prod only, gated at
// mailpit.ops.<host> (dashboard:mailpit#view) and the SPA renders behind a real AAL2
// operator session. The sink holds every message the Kratos courier submits, so this
// origin is how an engineer reads a verification or recovery mail.
import { type APIRequestContext, expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";
import { ADMIN } from "../fixtures/identities";
import { startRecovery } from "../fixtures/kratos";

const MAILPIT = `${opsURL("mailpit")}/`;

test.describe("mailpit ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("mailpit");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("mailpit");
  });

  test("gated: AAL2 operator passes the dashboard:mailpit#view grant", async () => {
    await expectOperatorAllowed("mailpit");
  });

  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("the Mailpit SPA paints at the subdomain root", async ({ page }) => {
      await page.goto(MAILPIT);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      // Mailpit mounts its SPA at root and titles the document "Mailpit".
      await expect(page).toHaveTitle(/mailpit/i, { timeout: 30_000 });
    });
  });

  // The sink is evidence, not decoration. Mailpit exposes an HTTP API over the same
  // store the UI reads (ADR-0307), which is what lets a test assert delivery instead
  // of leaving it to someone looking at a screen. It sits behind the same operator
  // gate as the UI, so these read it through the edge with the operator session.
  test.describe("holds a readable recovery mail", () => {
    test.use({ storageState: OPERATOR_STATE });

    test("the code in the body matches the code in the subject", async ({ page, browser }) => {
      // Three waits stack here and the 60s default covers none of them: gotoFlow
      // retries for up to 90s because the auth routes are rate-limited (10/min,
      // and the suite is one source IP), then the courier drains its queue on its
      // own schedule before anything reaches the sink.
      test.setTimeout(240_000);

      const recipient = ADMIN.email;
      // Only messages newer than this count. The sink is shared with whoever is
      // using the cluster, so the run neither clears it nor reads a message an
      // earlier run left behind.
      const since = Date.now();

      // Recovery refuses to start for an identity that already has a session —
      // Kratos 302s the init straight to the return URL — so it is driven in a
      // context with no session, while the operator session on `page` reads the
      // sink. Both context options are load-bearing and neither is inherited the
      // way it first appears:
      //
      //   - storageState is emptied explicitly. A bare browser.newContext() under
      //     this describe's `test.use` still arrives holding ory_kratos_session,
      //     and the symptom is the landing page instead of the form.
      //   - ignoreHTTPSErrors is restated because the project's `use` block does
      //     not reach a hand-made context, and the local wildcard cert is
      //     self-signed. Without it the page renders blank.
      const anon = await browser.newContext({
        ignoreHTTPSErrors: true,
        storageState: { cookies: [], origins: [] },
      });
      try {
        await startRecovery(await anon.newPage(), recipient);
      } finally {
        await anon.close();
      }

      let found: SinkMessage | undefined;
      await expect(async () => {
        const message = await newestTo(page.request, recipient, since);
        expect(message, `no message for ${recipient} reached the sink`).not.toBeNull();
        found = message ?? undefined;
      }).toPass({ timeout: 60_000, intervals: [1_000, 2_000, 5_000] });

      const message = found as SinkMessage;
      expect(message.To.map((t) => t.Address)).toContain(recipient);

      // Kratos puts the code in the subject AND in the body. Asserting only that a
      // message arrived passes just as happily on an empty or malformed template,
      // and that is the fault class production can show nobody: it keeps no store,
      // so a broken template is only ever visible below it (ADR-0307).
      const subjectCode = message.Subject.match(/\b(\d{6})\b/)?.[1];
      expect(subjectCode, `no six-digit code in subject: ${message.Subject}`).toBeDefined();

      const detail = await page.request.get(`${opsURL("mailpit")}/api/v1/message/${message.ID}`);
      expect(detail.status()).toBe(200);
      const { Text } = (await detail.json()) as { Text: string };
      expect(Text).toContain(subjectCode);

      // The sender is deliberately NOT asserted. `from_address` is unset, so Kratos
      // sends as its own upstream default rather than as the platform, and pinning
      // that here would make a configuration gap look like a decision.
    });
  });
});

// A message as the list and search endpoints return it.
type SinkMessage = {
  ID: string;
  Subject: string;
  Created: string;
  From: { Address: string };
  To: Array<{ Address: string }>;
};

// newestTo returns the most recent message addressed to `address` that arrived at or
// after `since`, or null while none has.
async function newestTo(
  request: APIRequestContext,
  address: string,
  since: number,
): Promise<SinkMessage | null> {
  const query = encodeURIComponent(`to:${address}`);
  const res = await request.get(`${opsURL("mailpit")}/api/v1/search?query=${query}`);
  expect(res.status()).toBe(200);
  const { messages } = (await res.json()) as { messages: SinkMessage[] };
  return (
    messages
      .filter((m) => Date.parse(m.Created) >= since)
      .sort((a, b) => Date.parse(b.Created) - Date.parse(a.Created))[0] ?? null
  );
}
