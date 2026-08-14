// Who may read an order (ADR-0003, ADR-0304). The rule under test is the one that
// is invisible until someone tries: an unguessable identifier is not an access
// control, so holding an order id grants nothing. The order's own buyer reads it
// through the `order#read` tuple the checkout saga writes; another signed-in buyer
// is refused; a caller with no session is refused before any lookup happens.
//
// This has to be an e2e. The decision spans the edge (which injects the identity
// headers), the orders service (which calls the Checker), OpenFGA (which resolves
// the tuple) and the Temporal saga (which wrote it) — a handler test can assert the
// call is made, and nothing below this level can assert the answer is right.
import { expect, test, type BrowserContext, type Browser } from "@playwright/test";
import { BASE_URL } from "../fixtures/env";
import { passwordLogin, register } from "../fixtures/kratos";
import { portForward } from "../fixtures/kube";

const KRATOS_ADMIN = "http://127.0.0.1:4434";

// Unique per run: Kratos enforces address uniqueness globally.
const stamp = Date.now();
const BUYER = { email: `buyer-a-${stamp}@e2e.localtest.me`, password: "Harbour-Sable-Quince-71!" };
const OTHER = { email: `buyer-b-${stamp}@e2e.localtest.me`, password: "Lantern-Pumice-Drift-83!" };

// A signed-in browser context. `storageState: undefined` is load-bearing: a new
// context inherits the project's stored operator/user state otherwise, and Kratos
// bounces an already-authenticated visitor off the registration flow.
async function signedIn(browser: Browser, who: { email: string; password: string }) {
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true, storageState: undefined });
  const page = await ctx.newPage();
  await register(page, who.email, who.password);
  await passwordLogin(page, who.email, who.password);
  await page.close();
  return ctx;
}

// checkout places an order as whoever this context is signed in as.
//
// It retries because the org is eventually consistent: registration only ENQUEUES
// the RegisterUser workflow, and an order belongs to the org the buyer acts through
// (ADR-0304), so a checkout that lands before the workflow finishes is correctly
// refused. Retrying is the assertion that it becomes possible, not a sleep.
async function checkout(ctx: BrowserContext, productId: string): Promise<string> {
  let orderId = "";
  await expect(async () => {
    const res = await ctx.request.post(`${BASE_URL}/api/orders`, {
      data: { product_id: productId, quantity: 1 },
      // Required: a checkout is idempotent on this key (ADR-0003). One per attempt,
      // so the retry loop below places one order rather than replaying the first.
      headers: { "Idempotency-Key": `authz-e2e-${Date.now()}-${Math.random()}` },
    });
    expect(res.status(), "checkout accepted once the buyer's org exists").toBe(202);
    orderId = ((await res.json()) as { run_id: string }).run_id;
  }).toPass({ timeout: 60_000 });
  return orderId;
}

test.describe("order read authorization", () => {
  // Signed in as nobody: these contexts are created per test from a clean state.
  test.use({ storageState: undefined });

  let buyer: BrowserContext;
  let other: BrowserContext;
  let anonymous: BrowserContext;
  let orderId = "";

  test.beforeAll(async ({ browser }) => {
    anonymous = await browser.newContext({ ignoreHTTPSErrors: true, storageState: undefined });

    // The catalog is the one collection that reads without a session, which is why
    // it can be fetched from the context that has none.
    const products = (await (
      await anonymous.request.get(`${BASE_URL}/api/products`)
    ).json()) as Array<{ id: string }>;
    expect(products.length, "the catalog has a product to buy").toBeGreaterThan(0);

    buyer = await signedIn(browser, BUYER);
    other = await signedIn(browser, OTHER);

    orderId = await checkout(buyer, products[0].id);
    expect(orderId, "checkout returned an order id").toMatch(/^order_[0-7][0-9abcdefghjkmnpqrstvwxyz]{25}$/);
  });

  test.afterAll(async () => {
    await Promise.all([buyer?.close(), other?.close(), anonymous?.close()]);

    // Delete both buyers. A registration per run that nobody removes is not
    // tidiness: the admin console's identity changelist paginates client-side, so
    // accumulated test identities push the seeded pair off the first page and
    // admin.spec's "seeded identities appear" assertion starts failing — a failure
    // that reads as a broken console and is actually this spec's litter.
    const pf = await portForward("ory-kratos-admin", 4434, 80);
    try {
      for (const who of [BUYER, OTHER]) {
        const res = await fetch(
          `${KRATOS_ADMIN}/admin/identities?credentials_identifier=${encodeURIComponent(who.email)}`,
        );
        if (!res.ok) {
          continue;
        }
        const list = (await res.json()) as Array<{ id: string; traits?: { email?: string } }>;
        const hit = list.find((i) => i.traits?.email === who.email);
        if (hit) {
          await fetch(`${KRATOS_ADMIN}/admin/identities/${hit.id}`, { method: "DELETE" });
        }
      }
    } finally {
      pf.stop();
    }
  });

  // The row is written by the saga's first activity, so it appears just after the
  // 202 rather than with it.
  test("its own buyer reads it", async () => {
    await expect(async () => {
      const res = await buyer.request.get(`${BASE_URL}/api/orders/${orderId}`);
      expect(res.status()).toBe(200);
      expect(((await res.json()) as { id: string }).id).toBe(orderId);
    }).toPass({ timeout: 60_000 });
  });

  test("another signed-in buyer holding the identifier is refused", async () => {
    const res = await other.request.get(`${BASE_URL}/api/orders/${orderId}`);
    expect(res.status(), "an order id is not a capability").toBe(403);
  });

  test("a caller with no session is refused", async () => {
    const res = await anonymous.request.get(`${BASE_URL}/api/orders/${orderId}`);
    expect(res.status()).toBe(401);
  });

  // The back-office collections are operator-only; a product user is not one.
  test("a buyer cannot list every order", async () => {
    const res = await buyer.request.get(`${BASE_URL}/api/orders`);
    expect(res.status()).toBe(403);
  });
});
