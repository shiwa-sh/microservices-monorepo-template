// Web vitals and per-route bundle budgets as a merge gate (ADR-0400).
//
// A .cjs config rather than the JSON one it replaces: the two notes below are what
// a JSON file could not carry, and both name a trap that has already been walked
// into once. The origin is a variable for the same reason LHCI_URL exists — the
// budgets are a property of the build, not of the machine serving it.
const ORIGIN = process.env.LHCI_URL ?? "https://dev.localtest.me:8443";

// The gate itself. LCP/CLS/TBT are ADR-0400's numbers; TBT stands in for INP,
// which is a field metric a navigation run has no interaction to measure.
const ASSERTIONS = {
  "largest-contentful-paint": ["error", { maxNumericValue: 2500 }],
  "cumulative-layout-shift": ["error", { maxNumericValue: 0.1 }],
  "total-blocking-time": ["error", { maxNumericValue: 200 }],
  "performance-budget": "error",
  "unused-javascript": "off",
  "uses-long-cache-ttl": "off",
  "csp-xss": "off",
  "is-on-https": "off",
};

module.exports = {
  ci: {
    collect: {
      // The app route is /auth/register. Kratos' FLOW is named "registration" and
      // the two are easy to confuse — /auth/registration is a 404, and lighthouse
      // reports it as "unable to reliably load the page", naming neither the route
      // nor the typo. Same trap as test/e2e/fixtures/env.ts guards for the specs.
      url: [`${ORIGIN}/`, `${ORIGIN}/auth/login`, `${ORIGIN}/auth/register`],
      numberOfRuns: 3,
      chromePath: "",
      settings: {
        chromeFlags: "--ignore-certificate-errors --no-sandbox",
        formFactor: "mobile",
        screenEmulation: {
          mobile: true,
          width: 412,
          height: 823,
          deviceScaleFactor: 1.75,
          disabled: false,
        },
        throttlingMethod: "simulate",
        budgetsPath: "./lighthouse-budgets.json",
      },
    },
    assert: {
      // median, not lhci's default optimistic: a budget answers "what does a
      // typical load cost", and optimistic answers "what does the best of three
      // cost", which no user gets and no regression has to clear.
      //
      // It only bites on the landing page. lhci groups runs by the URL that was
      // actually loaded (`_.groupBy(lhrs, lhr => lhr.finalUrl)`), and the two auth
      // routes 307 to the Kratos flow init, which stamps a fresh `?flow=<uuid>` per
      // run — so their three runs are three groups of one, and there is nothing to
      // take a median of. `assertMatrix` does not help: its patterns choose which
      // assertions apply to a group, after the grouping has happened.
      aggregationMethod: "median",
      assertions: ASSERTIONS,
    },
    upload: { target: "filesystem", outputDir: "./lighthouse-report" },
  },
};
