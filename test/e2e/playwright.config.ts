// One config, one runner (ADR-0018). The Go/shell preflight runs first as a
// failure localiser (globalSetup); the `setup` project provisions the committed
// test identities and saves their sessions; the `platform` project is the browser
// acceptance gauge — operator dashboards rendered behind a real AAL2 session.
import { defineConfig } from "@playwright/test";
import { BASE_URL } from "./fixtures/env";

export default defineConfig({
  testDir: ".",
  // Heavy SPA dashboards + a shared identity store: run serial for determinism.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  // Preflight readiness gate (Go) — reads "infra down" vs "app broken" before any
  // browser starts. Skip with E2E_SKIP_PREFLIGHT=1 when iterating on a known-good
  // cluster.
  globalSetup: "./preflight/preflight.setup.ts",
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: BASE_URL,
    ignoreHTTPSErrors: true, // local wildcard cert is self-signed
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    { name: "setup", testMatch: /fixtures\/auth\.setup\.ts/ },
    { name: "platform", testDir: "platform", dependencies: ["setup"] },
  ],
});
