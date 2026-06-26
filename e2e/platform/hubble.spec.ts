// Hubble UI operator dashboard (plan.md #8) — gated at hubble.ops.<host>
// (dashboard:hubble#view) and the flow-map renders behind a real AAL2 session.
// Hubble's React Router only runs at an origin ROOT (ADR-0003), which the
// {tool}.ops.<host> topology gives it.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectOperatorForbidden,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

const HUBBLE = `${opsURL("hubble")}/`;

test.describe("hubble ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("hubble");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("hubble");
  });

  test("gated: AAL2 operator passes the dashboard:hubble#view grant", async () => {
    await expectOperatorAllowed("hubble");
  });

  // Per-tool authz is real, not coarse: the operator holds grafana/temporal/hubble
  // but NOT minio, so the same AAL2 session is still forbidden there (ADR-0010).
  test("gated: operator without a minio grant is still forbidden", async () => {
    await expectOperatorForbidden("minio");
  });

  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("the flow-map paints at the subdomain root", async ({ page }) => {
      await page.goto(HUBBLE);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      // hubble-ui sets the document title to "Hubble" and mounts its SPA at root.
      await expect(page).toHaveTitle(/Hubble/, { timeout: 30_000 });
    });
  });
});
