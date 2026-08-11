// Grafana operator dashboard (ADR-0017) — the staged localiser chain: reachable
// (preflight), gated (edge authz), login (the real interactive flow, @smoke), and
// renders (the gauge — the dashboard paints behind a real AAL2 session).
import fs from "node:fs";
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, OPERATOR_TOTP_FILE, opsURL } from "../fixtures/env";
import { OPERATOR } from "../fixtures/identities";
import { operatorLogin } from "../fixtures/kratos";

const GRAFANA = `${opsURL("grafana")}/`;

test.describe("grafana ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("grafana");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("grafana");
  });

  test("gated: AAL2 operator passes the dashboard:grafana#view grant", async () => {
    await expectOperatorAllowed("grafana");
  });

  // The real end-to-end auth flow from a fresh context (not a pre-seeded session).
  test("login: operator authenticates through Kratos (incl. AAL2 step-up) @smoke", async ({
    browser,
  }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    // Hitting the dashboard unauthenticated redirects an html browser to Kratos.
    await page.goto(GRAFANA);
    await expect(page).toHaveURL(/\/auth\/login/);
    // Log in for real: password (AAL1) then the TOTP second factor (AAL2).
    const secret = fs.readFileSync(OPERATOR_TOTP_FILE, "utf8").trim();
    await operatorLogin(page, OPERATOR.email, OPERATOR.password, secret);
    // Authenticated, the dashboard now renders.
    await page.goto(GRAFANA);
    await expect(page).toHaveTitle(/Grafana/, { timeout: 30_000 });
    await ctx.close();
  });

  // The gauge: reusing the saved AAL2 session, the UI paints.
  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("a dashboard view paints @smoke", async ({ page }) => {
      await page.goto(GRAFANA);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      await expect(page).toHaveTitle(/Grafana/, { timeout: 30_000 });
    });
  });
});
