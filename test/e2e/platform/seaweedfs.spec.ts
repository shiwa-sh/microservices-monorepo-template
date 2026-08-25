// SeaweedFS admin operator dashboard (ADR-0306, ADR-0207) — non-prod object store,
// gated at seaweedfs.ops.<host> (dashboard:seaweedfs#view) and rendering behind a
// real AAL2 operator session. The admin UI carries no login of its own, so the
// edge gate is the only thing this asserts on the way in.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

const SEAWEEDFS = `${opsURL("seaweedfs")}/`;

test.describe("seaweedfs ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("seaweedfs");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("seaweedfs");
  });

  test("gated: AAL2 operator passes the dashboard:seaweedfs#view grant", async () => {
    await expectOperatorAllowed("seaweedfs");
  });

  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("the admin UI paints at the subdomain root", async ({ page }) => {
      await page.goto(SEAWEEDFS);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      // The dashboard itself, titled "SeaweedFS Admin" (verified against 4.41):
      // with `-admin.password` unset there is no login between the edge gate and
      // the store's administration.
      await expect(page).toHaveTitle(/SeaweedFS Admin/, { timeout: 30_000 });
    });
  });
});
