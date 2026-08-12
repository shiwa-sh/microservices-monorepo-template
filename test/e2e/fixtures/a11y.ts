// Accessibility scanning for the e2e suite (ADR-0400, ADR-0601). One toolchain:
// axe runs inside the browser Playwright already drives, so there is no second
// runner and no second definition of "the app is up".
//
// The gate is `serious` or `critical` only. `moderate` and `minor` are reported in
// the failure message when something else already failed, but they do not fail a
// merge on their own: axe's lower impact levels include findings that are correct
// by construction on a page fragment, and a gate that cries wolf is a gate people
// learn to re-run until it passes.
//
// A green axe run is NOT WCAG conformance (ADR-0400): it catches the
// machine-checkable subset. Meaningful alt text, reading order, and whether a flow
// is completable by keyboard are not detectable by a tool. This gate prevents
// regressions in the part a machine can see, and nothing more.
import AxeBuilder from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";

/** Impact levels that fail a merge. */
const BLOCKING = new Set(["serious", "critical"]);

/**
 * WCAG 2.2 AA is the target across every route group, so the tag set is the AA
 * ladder plus the two "best practice"-free WCAG 2.2 additions. `best-practice` is
 * deliberately absent: it is axe's opinion, not a success criterion.
 */
const WCAG22AA = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

export interface ScanOptions {
  /** CSS selector to scope the scan to, e.g. one kitchen-sink section. */
  include?: string;
  /**
   * Selectors to exclude. Vendored islands with their own theme are not claimed
   * (ADR-0400): Scalar's rendered console is the standing example.
   */
  exclude?: string[];
}

/**
 * Scan the current page and fail on any serious or critical violation.
 * `label` names the surface in the failure message, because "3 violations" without
 * a surface is a message that costs a bisect to act on.
 */
export async function expectNoA11yViolations(
  page: Page,
  label: string,
  options: ScanOptions = {},
): Promise<void> {
  let builder = new AxeBuilder({ page }).withTags(WCAG22AA);
  if (options.include) {
    builder = builder.include(options.include);
  }
  for (const selector of options.exclude ?? []) {
    builder = builder.exclude(selector);
  }

  const results = await builder.analyze();
  const blocking = results.violations.filter((v) => BLOCKING.has(v.impact ?? ""));
  if (blocking.length === 0) {
    return;
  }

  const detail = blocking
    .map((v) => {
      const where = v.nodes
        .slice(0, 3)
        .map((n) => n.target.join(" "))
        .join("\n      ");
      return `  [${v.impact}] ${v.id}: ${v.help}\n    ${v.helpUrl}\n    at: ${where}`;
    })
    .join("\n");

  expect(
    blocking,
    `${label}: ${blocking.length} serious/critical WCAG 2.2 AA violation(s)\n${detail}`,
  ).toHaveLength(0);
}

/**
 * Every `<section>` on the kitchen-sink page, by its heading text. A primitive's
 * conformance is proven once here rather than re-proven in every consumer
 * (ADR-0400), so the scan is per section: a failure names the primitive rather
 * than the page.
 */
export async function kitchenSinkSections(page: Page): Promise<string[]> {
  return page.locator("main section h2").allInnerTexts();
}
