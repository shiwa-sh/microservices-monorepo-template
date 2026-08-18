// Mailpit viewer operator dashboard (ADR-0307, ADR-0306) — non-prod only, gated at
// mailpit.ops.<host> (dashboard:mailpit#view) and the SPA renders behind a real AAL2
// operator session. The sink holds every message the Kratos courier submits, so this
// origin is how an engineer reads a verification or recovery mail.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

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
});
