// Argo CD operator dashboard (ADR-0004) — the GitOps control plane, gated at
// argocd.ops.<host> (dashboard:argocd#view) and rendering behind a real AAL2 operator
// session. argocd-server runs with server.insecure (TLS terminates at Traefik);
// its own UI keeps a login behind the Kratos gate, so the gauge asserts the SPA
// shell paints, not a post-login view.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";

const ARGO = `${opsURL("argocd")}/`;

test.describe("argocd ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("argocd");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("argocd");
  });

  test("gated: AAL2 operator passes the dashboard:argocd#view grant", async () => {
    await expectOperatorAllowed("argocd");
  });

  test.describe("renders behind AAL2", () => {
    test.use({ storageState: OPERATOR_STATE });
    test("the Argo CD SPA paints at the subdomain root", async ({ page }) => {
      await page.goto(ARGO);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      // argocd/argocd-ui mounts at root and titles the document "Argo CD".
      await expect(page).toHaveTitle(/Argo CD/, { timeout: 30_000 });
    });
  });
});
