// The one composed route with a baseline (ADR-0601).
//
// The login page is the exception to "primitives are pinned, pages are not": it is
// the first thing a user sees, it is rendered from the identity provider's own
// flow rather than from our data, and it holds nothing that changes between runs.
// Every other product route composes live data, where a baseline pins the fixture
// rather than the design.
import { expect, test } from "@playwright/test";
import { LOGIN_URL } from "../fixtures/env";

test.describe("visual regression @visual", () => {
  test("the login page matches its baseline", async ({ page }) => {
    await page.goto(LOGIN_URL);
    await expect(page.getByRole("button", { name: /sign in|log in/i })).toBeVisible();

    await expect(page).toHaveScreenshot("auth-login.png", {
      animations: "disabled",
      fullPage: true,
      maxDiffPixelRatio: 0.01,
    });
  });
});
