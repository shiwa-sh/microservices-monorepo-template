// MinIO console operator dashboard (ADR-0011, ADR-0016) — non-prod object store,
// gated at minio.ops.<host> (dashboard:minio#view) and rendering behind a real
// AAL2 operator session. Like Grafana, the console keeps its own login behind the
// Kratos gate, so the gauge asserts its shell paints, not a post-login view.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

const MINIO = `${opsURL("minio")}/`;

test.describe("minio ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("minio");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("minio");
  });

  test("gated: AAL2 operator passes the dashboard:minio#view grant", async () => {
    await expectOperatorAllowed("minio");
  });

  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("the console shell paints at the subdomain root", async ({ page }) => {
      await page.goto(MINIO);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      // The MinIO Console SPA mounts at root and sets its document title.
      await expect(page).toHaveTitle(/MinIO/, { timeout: 30_000 });
    });
  });
});
