// The browser fingerprint's rules, pinned (ADR-0503).
//
// These mirror the Go tests in libs/go/observability, because the two sides must
// normalise identically: a rule that drifts on one side splits one fault into
// two dashboard rows, and nothing else in the pipeline can see that happen.
import { describe, expect, test } from "bun:test";

import { appFrames, fingerprint, normalise } from "./index";

const HEX8 = /^[0-9a-f]{8}$/;

describe("normalise", () => {
  test("an entity id becomes a placeholder", () => {
    expect(normalise("order order_01kztmx9e0fq1r13w5d1aerqw6 not found")).toBe(
      "order order_<x> not found",
    );
  });

  test("digit runs become a placeholder", () => {
    expect(normalise("retry 3 of 5")).toBe("retry <n> of <n>");
  });

  // Words that happen to be hex are words, not values.
  test("short words survive", () => {
    expect(normalise("deadlock detected")).toBe("deadlock detected");
    expect(normalise("add failed")).toBe("add failed");
  });

  test("a message with no varying part is unchanged", () => {
    expect(normalise("the pool is closed")).toBe("the pool is closed");
  });
});

describe("appFrames", () => {
  const stack = [
    "Error: boom",
    "    at placeOrder (https://app.example.com/_next/static/chunks/page.js:12:34)",
    "    at onSubmit (https://app.example.com/_next/static/chunks/node_modules/react-dom.js:9:1)",
    "    at handler (https://app.example.com/_next/static/chunks/panel.js:88:2)",
  ].join("\n");

  test("keeps first-party frames in order", () => {
    expect(appFrames(stack)).toEqual(["placeOrder", "handler"]);
  });

  test("drops vendor frames", () => {
    expect(appFrames(stack)).not.toContain("onSubmit");
  });

  test("an absent stack yields no frames", () => {
    expect(appFrames(undefined)).toEqual([]);
  });
});

describe("fingerprint", () => {
  const base = { name: "TypeError", message: "order order_1 not found", stack: "at f (a.js:1:1)" };

  test("is stable for the same fault", () => {
    expect(fingerprint(base)).toBe(fingerprint({ ...base }));
  });

  // The property the normaliser exists for: an id must not mint a fault.
  test("ignores the varying part of the message", () => {
    const other = { ...base, message: "order order_2 not found" };
    expect(fingerprint(other)).toBe(fingerprint(base));
  });

  test("separates distinct faults", () => {
    const other = { ...base, message: "the pool is closed" };
    expect(fingerprint(other)).not.toBe(fingerprint(base));
  });

  // A line-number change is a cosmetic edit and must not move the fault.
  test("ignores line and column numbers", () => {
    const moved = { ...base, stack: "at f (a.js:99:7)" };
    expect(fingerprint(moved)).toBe(fingerprint(base));
  });

  test("is short and hex", () => {
    expect(fingerprint(base)).toMatch(HEX8);
  });
});
