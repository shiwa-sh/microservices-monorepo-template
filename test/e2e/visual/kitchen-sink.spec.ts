// Component visual regression against committed baselines (ADR-0601, ADR-0400).
//
// The kitchen sink is the subject because it is where every primitive is composed
// once (ADR-0400): a primitive's rendering is pinned here rather than re-pinned in
// every route that uses it, so a diff names the primitive rather than the page.
//
// Section by section rather than one full-page shot. A single baseline for the
// whole page turns any change to any primitive into one enormous diff, which is
// the shape a reviewer approves without reading.
//
// THE BASELINES ARE PRODUCED BY THE FIRST RUN against `cluster:up full` and
// committed with the change that introduced them; an intentional UI change updates
// them in the same pull request. A run with no baseline fails rather than passing
// silently, which is Playwright's default and the behaviour this gate wants.
import { expect, test } from "@playwright/test";
import { kitchenSinkSections } from "../fixtures/a11y";
import { BASE_URL, USER_STATE } from "../fixtures/env";

test.describe("visual regression @visual", () => {
  test.use({ storageState: USER_STATE });

  test("every kitchen-sink section matches its baseline", async ({ page }) => {
    await page.goto(`${BASE_URL}/devportal/kitchen-sink`);
    await expect(page.getByRole("heading", { name: "UI kitchen sink" })).toBeVisible();

    const sections = await kitchenSinkSections(page);
    // An empty page would pass vacuously, and a vacuous pass on a regression gate
    // is worse than no gate: it reports coverage that does not exist.
    expect(sections.length, "kitchen sink rendered no primitive sections").toBeGreaterThan(0);

    for (const [index, title] of sections.entries()) {
      const section = page.locator(`main section:nth-of-type(${index + 1})`);
      await expect(section, `kitchen sink § ${title}`).toHaveScreenshot(
        `kitchen-sink-${index + 1}.png`,
        {
          // Animations mid-flight are the single largest source of a diff nobody
          // introduced.
          animations: "disabled",
          // Antialiasing differs by a pixel or two between runs on the same
          // machine; anything larger is a real change.
          maxDiffPixelRatio: 0.01,
        },
      );
    }
  });
});
