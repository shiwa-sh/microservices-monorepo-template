// The consent gate's rules, pinned (ADR-0700).
//
// Each test is a legal position rather than a preference, which is why they are
// asserted rather than left to the caller: silence is not consent, a prior signal
// is honoured without prompting, and a changed purpose is a new question.
import { describe, expect, test } from "bun:test";

import { type ConsentDecision, gpcDecision, mayEmitMarketing, shouldPrompt } from "./consent";

const VERSION = "2026-08-01";
const granted: ConsentDecision = { state: "granted", source: "control", purposeVersion: VERSION };

describe("mayEmitMarketing", () => {
  test("an explicit grant permits emission", () => {
    expect(mayEmitMarketing(granted)).toBe(true);
  });

  // The load-bearing one: a visitor who has not answered has not consented.
  test("no decision is not a grant", () => {
    expect(mayEmitMarketing(null)).toBe(false);
  });

  test("withdrawal stops emission", () => {
    expect(mayEmitMarketing({ ...granted, state: "withdrawn" })).toBe(false);
  });

  test("a refusal stops emission", () => {
    expect(mayEmitMarketing({ ...granted, state: "refused" })).toBe(false);
  });

  // A grant to an older purpose text still permits emission; what it triggers is a
  // re-prompt, not a silent stop. Stopping would lose data the visitor agreed to.
  test("a grant to an older purpose still permits emission", () => {
    expect(mayEmitMarketing({ ...granted, purposeVersion: "2026-01-01" })).toBe(true);
  });
});

describe("shouldPrompt", () => {
  test("an unanswered visitor is prompted", () => {
    expect(shouldPrompt(null, VERSION)).toBe(true);
  });

  test("a current grant is not re-asked", () => {
    expect(shouldPrompt(granted, VERSION)).toBe(false);
  });

  // A changed purpose is a new question (GDPR Art. 7(1)).
  test("a changed purpose re-asks", () => {
    expect(shouldPrompt({ ...granted, purposeVersion: "2026-01-01" }, VERSION)).toBe(true);
  });

  // Never re-ask someone who sent a prior signal, even across a purpose change.
  test("a GPC refusal is never re-asked", () => {
    expect(shouldPrompt(gpcDecision("2026-01-01"), VERSION)).toBe(false);
  });
});

describe("gpcDecision", () => {
  test("records a refusal rather than merely obeying one", () => {
    const decision = gpcDecision(VERSION);
    expect(decision.state).toBe("refused");
    expect(decision.source).toBe("gpc");
  });
});
