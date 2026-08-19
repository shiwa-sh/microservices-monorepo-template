// The generated schemas, exercised (ADR-0400, ADR-0601).
//
// These tests are deliberately about the GENERATOR rather than about zod. Each
// one asserts a property that a weaker generator would silently lose — and losing
// it produces a form that accepts what the API refuses, which is a defect nothing
// else in the pipeline can see.
import { describe, expect, test } from "bun:test";

import { createProductSchema, moneySchema } from "./catalog";
import { checkoutSchema, productIdSchema } from "./orders";
import { refundChargeSchema } from "./payment";

describe("generated zod schemas", () => {
  test("a valid body passes", () => {
    const parsed = createProductSchema.safeParse({
      name: "Widget",
      price: { amount: "10.00", currency: "EUR" },
    });
    expect(parsed.success).toBe(true);
  });

  test("a missing required member fails", () => {
    expect(createProductSchema.safeParse({ name: "Widget" }).success).toBe(false);
  });

  // The spec's `minLength: 1` survives. A generator that dropped constraints would
  // accept the empty string here, and the API would reject it.
  test("minLength survives", () => {
    const parsed = createProductSchema.safeParse({
      name: "",
      price: { amount: "10.00", currency: "EUR" },
    });
    expect(parsed.success).toBe(false);
  });

  // The prefixed-identifier pattern survives, ANCHORED. An unanchored regex would
  // accept any string merely containing an id.
  // biome-ignore-start lint/security/noSecrets: prefixed identifiers, not credentials
  test("an id pattern is anchored", () => {
    expect(productIdSchema.safeParse("product_01kztmx9e0fq1r13w5d1aerqw6").success).toBe(true);
    expect(productIdSchema.safeParse("order_01kztmx9e0fq1r13w5d1aerqw6").success).toBe(false);
    expect(productIdSchema.safeParse("xproduct_01kztmx9e0fq1r13w5d1aerqw6y").success).toBe(false);
  });

  test("currency must be three upper-case letters", () => {
    expect(moneySchema.safeParse({ amount: "1.00", currency: "eur" }).success).toBe(false);
  });

  // `minimum: 1` on an integer, and the integer-ness itself.
  test("an integer minimum survives", () => {
    const body = { product_id: "product_01kztmx9e0fq1r13w5d1aerqw6" };
    expect(checkoutSchema.safeParse({ ...body, quantity: 0 }).success).toBe(false);
    expect(checkoutSchema.safeParse({ ...body, quantity: 1.5 }).success).toBe(false);
    expect(checkoutSchema.safeParse({ ...body, quantity: 2 }).success).toBe(true);
  });
  // biome-ignore-end lint/security/noSecrets: prefixed identifiers, not credentials

  // A schema that references a component must be usable at import time. The
  // generated consts are not hoisted, so this is a real failure mode: the first
  // version of the generator emitted them alphabetically and orders/ threw a
  // ReferenceError on import.
  test("a body referencing a component parses", () => {
    expect(refundChargeSchema.safeParse({ reason: "duplicate" }).success).toBe(true);
    expect(refundChargeSchema.safeParse({ reason: "" }).success).toBe(false);
  });
});
