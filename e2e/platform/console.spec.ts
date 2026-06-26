// Lowdefy admin console ops dashboard (ADR-0012, ADR-0017). Same staged gauge as
// the other ops tools: gated at the edge (unauthenticated / AAL1 / AAL2 operator
// holding dashboard:console#view), then the Lowdefy app paints behind a real AAL2
// session.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

const CONSOLE = `${opsURL("console")}/`;

test.describe("console ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("console");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("console");
  });

  test("gated: AAL2 operator passes the dashboard:console#view grant", async () => {
    await expectOperatorAllowed("console");
  });

  // The gauge: reusing the saved AAL2 session, the Lowdefy admin renders.
  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("the admin dashboard paints @smoke", async ({ page }) => {
      await page.goto(CONSOLE);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      // dashboard.yaml renders a Markdown "# Platform admin" heading.
      await expect(
        page.getByRole("heading", { name: "Platform admin" }),
      ).toBeVisible({ timeout: 30_000 });
    });
  });
});
