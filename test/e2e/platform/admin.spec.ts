// Lowdefy admin console ops dashboard (ADR-0012, ADR-0017). Same staged gauge as
// the other ops tools: gated at the edge (unauthenticated / AAL1 / AAL2 operator
// holding dashboard:lowdefy#view), then the Lowdefy app paints behind a real AAL2
// session.
//
// Beyond "paints", the products suite drives a full create → list → edit → delete
// round-trip through the generated Django-admin pages. That exercise is deliberate:
// the console reaches each service east-west (bypassing the /api edge), so it only
// works when the service NetworkPolicy admits the lowdefy pod and the grid binds the
// response body (`_request: list.data`). A paint-only check (a static grid header is
// visible) passes even when the grid is empty because the request timed out — the
// exact regression this suite now guards against.
import { expect, test } from "@playwright/test";
import {
  expectAal1Forbidden,
  expectOperatorAllowed,
  expectUnauthenticatedDenied,
} from "../fixtures/dashboard";
import { OPERATOR_STATE, opsURL } from "../fixtures/env";
import { OPERATOR, USER } from "../fixtures/identities";
import { portForward } from "../fixtures/kube";

const CONSOLE = `${opsURL("lowdefy")}/`;
const CONSOLE_CREATE_OPERATOR = `${opsURL("lowdefy")}/createOperator`;
const TEST_OPERATOR_EMAIL = "new-op@e2e.localtest.me";

test.describe("lowdefy ops dashboard", () => {
  test("gated: unauthenticated is denied", async () => {
    await expectUnauthenticatedDenied("lowdefy");
  });

  test("gated: AAL1 product session is forbidden", async () => {
    await expectAal1Forbidden("lowdefy");
  });

  test("gated: AAL2 operator passes the dashboard:lowdefy#view grant", async () => {
    await expectOperatorAllowed("lowdefy");
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

  // The products resource wired to a live catalog service: a full CRUD round-trip
  // through the generated changelist / add / edit pages. This is the data gauge —
  // if the console cannot reach catalog, or the grid binds the HTTP envelope instead
  // of its body, the created row never appears and the test fails.
  test.describe("products CRUD", () => {
    test.use({ storageState: OPERATOR_STATE });

    test("create, list, edit and delete a product @smoke", async ({ page }) => {
      const name = `e2e-widget-${Date.now()}`;
      const renamed = `${name}-v2`;
      const admin = opsURL("lowdefy");

      // Add: the standalone create page (Django "add" form). On success it redirects
      // to the changelist, where the new row is the confirmation — the assertion an
      // empty grid (the NetworkPolicy / `.data` / write-auth bugs) would fail.
      await page.goto(`${admin}/products_new`);
      await page.getByLabel(/^name$/i).fill(name);
      await page.getByLabel(/price/i).fill("4200");
      await page.getByRole("button", { name: "Create", exact: true }).click();
      await expect(page).toHaveURL(/\/products$/, { timeout: 20_000 });
      await expect(page.getByText(name)).toBeVisible({ timeout: 30_000 });

      // Row-click opens the change page, prefilled from GET /products/{id}.
      await page.getByText(name).click();
      await expect(page).toHaveURL(/products_edit\?id=/, { timeout: 15_000 });
      await expect(page.getByLabel(/^name$/i)).toHaveValue(name, { timeout: 20_000 });

      // Edit and save; the success toast confirms the PUT resolved.
      await page.getByLabel(/^name$/i).fill(renamed);
      await page.getByRole("button", { name: "Save", exact: true }).click();
      await expect(page.getByText("Saved changes")).toBeVisible({ timeout: 20_000 });

      // The changelist reflects the edit: renamed present, original gone.
      await page.goto(`${admin}/products`);
      await expect(page.getByText(renamed)).toBeVisible({ timeout: 30_000 });
      await expect(page.getByText(name, { exact: true })).toHaveCount(0);

      // Delete from the change page redirects to the changelist, row removed.
      await page.getByText(renamed).click();
      await expect(page).toHaveURL(/products_edit\?id=/, { timeout: 15_000 });
      await page.getByRole("button", { name: "Delete", exact: true }).click();
      await expect(page).toHaveURL(/\/products$/, { timeout: 15_000 });
      await expect(page.getByText(renamed)).toHaveCount(0);
    });
  });

  // The identities resource wired to Kratos through authz: the changelist and edit
  // page (ADR-0012). Unlike products, the rows come from Kratos admin (GET
  // /admin/identities) via authz — a path that only works when authz can reach the
  // Kratos admin API (network-policies/30-ory.yaml) and the deployed authz image
  // actually serves /identities. A paint-only check would pass on an empty grid; this
  // asserts the seeded bootstrap identities are present — the exact "I open Identities
  // and see nothing" regression — then edits one to exercise the PUT write path
  // (authz gates writes on the console's operator X-User-Id → OpenFGA group:operator).
  test.describe("identities", () => {
    test.use({ storageState: OPERATOR_STATE });

    test("list shows seeded identities and an edit round-trips @smoke", async ({ page }) => {
      const admin = opsURL("lowdefy");

      // Changelist: the seeded operator and product user must both appear. An empty
      // grid — a stale authz image without /identities, a blocked Kratos admin call,
      // or the grid binding the HTTP envelope instead of `list.data` — fails here.
      await page.goto(`${admin}/identities`);
      await expect(page).not.toHaveURL(/\/auth\/login/);
      await expect(page.getByText(OPERATOR.email)).toBeVisible({ timeout: 30_000 });
      await expect(page.getByText(USER.email)).toBeVisible({ timeout: 30_000 });

      // Row-click opens the change page, prefilled from GET /identities/{id}.
      await page.getByText(OPERATOR.email).first().click();
      await expect(page).toHaveURL(/identities_edit\?id=/, { timeout: 15_000 });
      const nameField = page.getByLabel(/^name$/i);
      await expect(nameField).toBeVisible({ timeout: 20_000 });
      const original = await nameField.inputValue();
      const renamed = `e2e-op-${Date.now()}`;

      // Edit the name and save; the success toast confirms the PUT resolved — proof
      // the operator identity header and OpenFGA grant let the write through.
      await nameField.fill(renamed);
      await page.getByRole("button", { name: "Save", exact: true }).click();
      await expect(page.getByText("Saved changes")).toBeVisible({ timeout: 20_000 });

      // The changelist reflects the edit (the write actually persisted to Kratos).
      await page.goto(`${admin}/identities`);
      await expect(page.getByText(renamed)).toBeVisible({ timeout: 30_000 });

      // Revert so the shared operator identity is left as we found it. Only when there
      // was a name to restore: the form (and Kratos schema) reject an empty name, and
      // the seeded operator has none — a per-run unique rename left behind is harmless
      // since every assertion above keys on the email, never the name.
      if (original) {
        await page.getByText(OPERATOR.email).first().click();
        await expect(page).toHaveURL(/identities_edit\?id=/, { timeout: 15_000 });
        await page.getByLabel(/^name$/i).fill(original);
        await page.getByRole("button", { name: "Save", exact: true }).click();
        await expect(page.getByText("Saved changes")).toBeVisible({ timeout: 20_000 });
      }
    });
  });

  // Operator management: the generated createOperator page creates a Kratos
  // identity and grants group:operator membership in OpenFGA via server-side API
  // calls (authz POST /operators).
  test.describe("create operator", () => {
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

    test("createOperator page renders and form submits @smoke", async ({ page }) => {
      await page.goto(CONSOLE_CREATE_OPERATOR);
      // Wait for the page to paint — the submit button signals the form rendered.
      await page.getByRole("button", { name: "Create operator" }).waitFor({ timeout: 30_000 });
      // antd Form.Item lowercases labels in the rendered HTML; use case-insensitive match.
      await page.getByLabel(/email/i).fill(TEST_OPERATOR_EMAIL);
      await page.getByLabel(/password/i).fill("NewOp-e2e-Sessi0n!");
      await page.getByRole("button", { name: "Create operator" }).click();
      // The onClick SetState runs only if the request resolved, revealing the
      // generated success Alert — the admin-gen confirmation block.
      await expect(page.getByText(/Create operator succeeded/)).toBeVisible({
        timeout: 20_000,
      });
    });
  });

  // The remaining generated pages (tools/admin-gen). Behind the AAL2 operator
  // session each must paint a page-specific control — proof the Lowdefy page
  // rendered rather than redirecting to login or stalling on an empty shell.
  test.describe("generated pages render", () => {
    test.use({ storageState: OPERATOR_STATE });

    const pages: Array<{ path: string; control: string; role: "button" | "text" }> = [
      { path: "/products", control: "Add product", role: "button" }, // changelist add
      { path: "/products_new", control: "Create", role: "button" }, // add form
      { path: "/orgs", control: "Add org", role: "button" }, // changelist add
      { path: "/orders", control: "Total cents", role: "text" }, // list-only grid header
      { path: "/charges", control: "Amount cents", role: "text" }, // list-only grid header
      { path: "/refundCharge", control: "Refund charge", role: "button" }, // action form
      { path: "/cancelOrder", control: "Cancel order", role: "button" }, // action form
    ];

    for (const p of pages) {
      test(`${p.path} paints @smoke`, async ({ page }) => {
        await page.goto(`${opsURL("lowdefy")}${p.path}`);
        await expect(page).not.toHaveURL(/\/auth\/login/);
        const control =
          p.role === "button"
            ? page.getByRole("button", { name: p.control })
            : page.getByText(p.control).first();
        await expect(control).toBeVisible({ timeout: 30_000 });
      });
    }
  });
});
