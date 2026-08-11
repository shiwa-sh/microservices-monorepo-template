// Temporal Web UI operator dashboard (ADR-0017) — gated at temporal.ops.<host>
// (dashboard:temporal#view) and the namespace view renders behind a real AAL2
// operator session.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

const TEMPORAL = `${opsURL("temporal")}/`;

test.describe("temporal ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("temporal");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("temporal");
  });

  test("gated: AAL2 operator passes the dashboard:temporal#view grant", async () => {
    await expectOperatorAllowed("temporal");
  });

  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("the namespace view paints", async ({ page }) => {
      await page.goto(TEMPORAL);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      // temporalio/ui lands on the default namespace's workflows view, whose SPA
      // title is "Workflows | <namespace>".
      await expect(page).toHaveTitle(/Workflows|Temporal/, { timeout: 30_000 });
    });
  });
});
