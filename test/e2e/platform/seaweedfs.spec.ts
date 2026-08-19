// SeaweedFS admin operator dashboard (ADR-0306, ADR-0207) — non-prod object store,
// gated at seaweedfs.ops.<host> (dashboard:seaweedfs#view) and rendering behind a
// real AAL2 operator session. Like the MinIO console this replaced, the admin UI
// keeps its own login behind the Kratos gate, so the gauge asserts its shell
// paints, not a post-login view.
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
      // Root redirects to the UI's own login, which titles itself "SeaweedFS
      // Admin - Login" (verified against 4.41) — the shell painted, which is
      // what the edge gate having passed looks like.
      await expect(page).toHaveTitle(/SeaweedFS Admin/, { timeout: 30_000 });
    });
  });
});
