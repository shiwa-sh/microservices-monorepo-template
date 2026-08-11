// Headlamp k8s debug UI operator dashboard (ADR-0024) — opt-in, gated at
// headlamp.ops.<host> (dashboard:headlamp#view) and the SPA renders behind a real AAL2
// operator session. Read-only via the built-in `view` ClusterRole; the gauge
// asserts the shell paints, not any post-connect view.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

const HEADLAMP = `${opsURL("headlamp")}/`;

test.describe("headlamp ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("headlamp");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("headlamp");
  });

  test("gated: AAL2 operator passes the dashboard:headlamp#view grant", async () => {
    await expectOperatorAllowed("headlamp");
  });

  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("the Headlamp SPA paints at the subdomain root", async ({ page }) => {
      await page.goto(HEADLAMP);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      // headlamp mounts its SPA at root and titles the document "Headlamp".
      await expect(page).toHaveTitle(/Headlamp/, { timeout: 30_000 });
    });
  });
});
