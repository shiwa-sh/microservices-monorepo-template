// Playwright globalSetup: run the Go preflight readiness gate before any browser
// starts (ADR-0018). A non-zero exit aborts the run with "infra down" rather than
// letting every browser test fail opaquely. Set E2E_SKIP_PREFLIGHT=1 to bypass
// when iterating against a known-good cluster.
import { execFileSync } from "node:child_process";

export default function globalSetup(): void {
  if (process.env.E2E_SKIP_PREFLIGHT === "1") {
    console.log("preflight: skipped (E2E_SKIP_PREFLIGHT=1)");
    return;
  }
  // cwd is the e2e/ config dir.
  execFileSync("bash", ["preflight/preflight.sh"], { stdio: "inherit" });
}
