// The consent gate for marketing events (ADR-0700).
//
// ePrivacy Art. 5(3) attaches to STORING OR READING information on the visitor's
// device, not to processing in general — so the boundary this file enforces is a
// technical line rather than a legal judgement, and the split is designed to sit
// on it:
//
//   - Ops RUM — errors, web vitals, traces — stores nothing on the device. The
//     session id lives in memory for the page's lifetime. No Art. 5(3) trigger, so
//     no consent gate, which is why a visitor who refuses analytics still gets
//     working error reporting.
//   - `marketing.*` — journey, funnel, retention — is persistent, because the
//     questions span visits. That needs consent recorded before the first event.
//
// This is the FIRST of the two enforcement points. The analytics service is the
// second, and it exists because this one runs on a client the platform does not
// control.

/** The recorded decision. `unknown` is "has not been asked", which is not a refusal. */
export type ConsentState = "granted" | "withdrawn" | "refused" | "unknown";

/** Where a decision came from. `gpc` is a signal nobody was prompted for. */
export type ConsentSource = "control" | "gpc";

export type ConsentDecision = {
  state: ConsentState;
  source: ConsentSource;
  purposeVersion: string;
};

/**
 * Whether a `marketing.*` event may be emitted.
 *
 * Only an explicit grant permits it. `unknown` does not: a visitor who has not
 * answered has not consented, and treating silence as agreement is the thing the
 * rule exists to stop.
 */
export function mayEmitMarketing(decision: ConsentDecision | null): boolean {
  return decision?.state === "granted";
}

/**
 * Whether the visitor should be shown the consent control.
 *
 * A Global Privacy Control signal is honoured as a refusal AND suppresses the
 * prompt, which is the point: prompting someone who has already answered is the
 * dark pattern the rule exists to stop. So a `gpc` decision is never re-asked,
 * even though a `control` refusal could be revisited when the purpose changes.
 */
export function shouldPrompt(decision: ConsentDecision | null, purposeVersion: string): boolean {
  if (decision === null) {
    return !globalPrivacyControl();
  }
  if (decision.source === "gpc") {
    return false;
  }
  // Consent is to a STATED purpose, so a changed purpose text is a new question
  // rather than a continuing answer (GDPR Art. 7(1)).
  return decision.purposeVersion !== purposeVersion;
}

/**
 * The visitor's Global Privacy Control signal, read from the navigator.
 *
 * A prior signal, in the sense the rule means: it was sent before anything was
 * asked, and honouring it is what makes the absence of a prompt correct rather
 * than an omission.
 */
export function globalPrivacyControl(): boolean {
  if (typeof navigator === "undefined") {
    return false;
  }
  return (
    (navigator as Navigator & { globalPrivacyControl?: boolean }).globalPrivacyControl === true
  );
}

/**
 * The decision to record for a visitor who sent Global Privacy Control.
 *
 * It is written rather than merely obeyed, because Art. 7(1) asks for consent —
 * and the absence of it — to be demonstrable after the fact. A refusal nobody
 * recorded looks identical to a question nobody asked.
 */
export function gpcDecision(purposeVersion: string): ConsentDecision {
  return { state: "refused", source: "gpc", purposeVersion };
}
