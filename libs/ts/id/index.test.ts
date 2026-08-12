import { describe, expect, test } from "bun:test";
import { formatId, InvalidIdError, isId, isValidPrefix, parseId, prefixOf } from "./index";

// The published TypeID vectors, identical to the ones libs/go/id asserts. Checking
// both implementations against the STANDARD rather than against each other is what
// makes them interoperable — two implementations of the same private encoding agree
// with each other perfectly and with nothing else.
const VECTORS: ReadonlyArray<readonly [uuid: string, suffix: string]> = [
  ["00000000-0000-0000-0000-000000000000", "00000000000000000000000000"],
  ["00000000-0000-0000-0000-000000000001", "00000000000000000000000001"],
  ["01890a5d-ac96-774b-bcce-b302099a8057", "01h455vb4pex5vsknk084sn02q"],
  ["ffffffff-ffff-ffff-ffff-ffffffffffff", "7zzzzzzzzzzzzzzzzzzzzzzzzz"],
];

describe("TypeID spec vectors", () => {
  test.each(VECTORS)("encodes %s", (uuid, suffix) => {
    expect(formatId("thing", uuid)).toBe(`thing_${suffix}`);
  });

  test.each(VECTORS)("decodes %s", (uuid, suffix) => {
    expect(parseId(`thing_${suffix}`).uuid).toBe(uuid);
  });
});

describe("parseId", () => {
  test("round-trips through the wire form", () => {
    const wire = formatId("order", "01890a5d-ac96-774b-bcce-b302099a8057");
    const parsed = parseId(wire, "order");
    expect(parsed.prefix).toBe("order");
    expect(parsed.uuid).toBe("01890a5d-ac96-774b-bcce-b302099a8057");
  });

  // The whole reason the prefix exists.
  test("rejects the wrong type", () => {
    const order = formatId("order", "01890a5d-ac96-774b-bcce-b302099a8057");
    expect(() => parseId(order, "product")).toThrow(InvalidIdError);
  });

  test.each([
    ["no separator", "order01j8xk7m3q0000000000000000"],
    ["empty prefix", "_01j8xk7m3q0000000000000000"],
    ["short suffix", "order_01j8xk7m3q"],
    ["long suffix", "order_01j8xk7m3q00000000000000000"],
    ["excluded letter", "order_01j8xk7m3q000000000000000i"],
    ["uppercase suffix", "order_01J8XK7M3Q0000000000000000"],
    ["uppercase prefix", "Order_01j8xk7m3q0000000000000000"],
    ["digit in prefix", "order2_01j8xk7m3q0000000000000000"],
    // 26 characters hold 130 bits; a leading character above 7 does not fit in 128.
    ["overflows 128 bits", "order_81j8xk7m3q0000000000000000"],
  ])("rejects %s", (_name, value) => {
    expect(() => parseId(value)).toThrow(InvalidIdError);
  });
});

describe("isId and prefixOf", () => {
  const wire = formatId("product", "01890a5d-ac96-774b-bcce-b302099a8057");

  test("isId accepts a well-formed identifier", () => {
    expect(isId(wire)).toBe(true);
    expect(isId(wire, "product")).toBe(true);
    expect(isId(wire, "order")).toBe(false);
    expect(isId("nonsense")).toBe(false);
  });

  test("prefixOf names the type, or null", () => {
    expect(prefixOf(wire)).toBe("product");
    expect(prefixOf("nonsense")).toBeNull();
  });
});

describe("isValidPrefix", () => {
  test.each(["order", "product", "payment_method", "a"])("accepts %s", (p) => {
    expect(isValidPrefix(p)).toBe(true);
  });

  test.each([
    "",
    "Order",
    "order-item",
    "order1",
    "_order",
    "order_",
    "a".repeat(64),
  ])("rejects %s", (p) => {
    expect(isValidPrefix(p)).toBe(false);
  });
});

// UUIDv7 is time-ordered, and the encoding has to preserve that at the boundary or
// the reason for choosing v7 is lost the moment a value is serialised.
test("encoding preserves the ordering of the underlying value", () => {
  const earlier = formatId("order", "01890a5d-ac96-774b-bcce-b302099a8057");
  const later = formatId("order", "01890a5d-ac96-774b-bcce-b302099a8058");
  expect(earlier < later).toBe(true);
});
