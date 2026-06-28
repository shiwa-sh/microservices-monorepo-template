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
import { portForward } from "../fixtures/kube";

const CONSOLE = `${opsURL("console")}/`;
const CONSOLE_OPERATORS = `${opsURL("console")}/operators`;
const TEST_OPERATOR_EMAIL = "new-op@e2e.localtest.me";

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

  // Operator management: the Lowdefy "Operators" page creates a Kratos identity
  // and grants group:operator membership in SpiceDB via server-side API calls.
  test.describe("add operator", () => {
    test.use({ storageState: OPERATOR_STATE });

    test.afterAll(async () => {
      const pf = await portForward("ory-kratos-admin", 4434, 80);
      try {
        const res = await fetch(
          `http://127.0.0.1:4434/admin/identities?credentials_identifier=${encodeURIComponent(TEST_OPERATOR_EMAIL)}`,
        );
        if (!res.ok) return;
        const list = (await res.json()) as Array<{ id: string; traits?: { email?: string } }>;
        const hit = list.find((i) => i.traits?.email === TEST_OPERATOR_EMAIL);
        if (hit) {
          await fetch(`http://127.0.0.1:4434/admin/identities/${hit.id}`, { method: "DELETE" });
        }
      } finally {
        pf.stop();
      }
    });

    test("operators page renders and form submits @smoke", async ({ page }) => {
      await page.goto(CONSOLE_OPERATORS);
      // Wait for the page to paint — the submit button signals the form rendered.
      await page.getByRole("button", { name: "Add operator" }).waitFor({ timeout: 30_000 });
      // antd Form.Item lowercases labels in the rendered HTML; use case-insensitive match.
      await page.getByLabel(/email/i).fill(TEST_OPERATOR_EMAIL);
      await page.getByLabel(/password/i).fill("NewOp-e2e-Sessi0n!");
      await page.getByRole("button", { name: "Add operator" }).click();
      // SetState success:true makes the successMsg Title (h5) block visible.
      await expect(page.getByRole("heading", { name: "Operator added" })).toBeVisible({
        timeout: 20_000,
      });
    });
  });
});
