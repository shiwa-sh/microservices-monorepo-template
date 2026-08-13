// WCAG 2.2 AA regression gate for the first-party surfaces (ADR-0400, ADR-0601).
// A `serious` or `critical` violation fails the merge.
//
// Two scopes, and they answer different questions:
//
//   1. The kitchen-sink page, section by section. A primitive's conformance is
//      proven once here rather than re-proven in every consumer, so a failure names
//      the primitive rather than "the app".
//   2. The product journeys, whole-page. Contrast, heading structure, landmark
//      semantics and error association are composition decisions no primitive
//      library makes, so they are only visible on a composed route.
//
// Not scanned, and the exclusion is stated rather than assumed (ADR-0400): Scalar's
// rendered console and the vendored operator UIs (Lowdefy, Grafana, pgweb) are
// third-party surfaces behind an operator session, and AA is not claimed for them.
import { expect, test } from "@playwright/test";
import { expectNoA11yViolations, kitchenSinkSections } from "../fixtures/a11y";
import { BASE_URL, REGISTER_URL, USER_STATE } from "../fixtures/env";

// Scalar renders into its own container with its own theme; the route group AROUND
// it is in scope, the console itself is not.
const SCALAR = ".scalar-app, [data-scalar], #scalar-api-reference";

test.describe("accessibility @a11y", () => {
  test.describe("kitchen sink — every primitive", () => {
    test.use({ storageState: USER_STATE });

    test("each section is free of serious and critical violations", async ({ page }) => {
      await page.goto(`${BASE_URL}/devportal/kitchen-sink`);
      await expect(page.getByRole("heading", { name: "UI kitchen sink" })).toBeVisible();

      const sections = await kitchenSinkSections(page);
      // A page that renders no sections is a broken fixture, not a pass: the whole
      // point of this scope is that every primitive is covered, so an empty list
      // would make the test vacuously green.
      expect(sections.length, "kitchen sink rendered no primitive sections").toBeGreaterThan(0);

      for (const [index, title] of sections.entries()) {
        await expectNoA11yViolations(page, `kitchen sink § ${title}`, {
          include: `main section:nth-of-type(${index + 1})`,
        });
      }
    });
  });

  test.describe("product journeys", () => {
    test("landing", async ({ page }) => {
      await page.goto(BASE_URL);
      await expectNoA11yViolations(page, "(landing) /");
    });

    test("login", async ({ page }) => {
      await page.goto(`${BASE_URL}/auth/login`);
      await expect(page.locator("form")).toBeVisible();
      await expectNoA11yViolations(page, "(landing) /auth/login");
    });

    test("registration", async ({ page }) => {
      await page.goto(REGISTER_URL);
      await expect(page.locator("form")).toBeVisible();
      await expectNoA11yViolations(page, "(landing) /auth/register");
    });

    test.describe("authenticated", () => {
      test.use({ storageState: USER_STATE });

      test("panel", async ({ page }) => {
        await page.goto(`${BASE_URL}/panel`);
        await expectNoA11yViolations(page, "(panel) /panel");
      });

      test("checkout", async ({ page }) => {
        await page.goto(`${BASE_URL}/panel/checkout`);
        await expectNoA11yViolations(page, "(panel) /panel/checkout");
      });

      test("devportal", async ({ page }) => {
        await page.goto(`${BASE_URL}/devportal`);
        await expectNoA11yViolations(page, "(devportal) /devportal", { exclude: [SCALAR] });
      });
    });
  });
});
