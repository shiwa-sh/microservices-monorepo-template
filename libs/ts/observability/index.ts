// Error fingerprinting, browser side (ADR-0503).
//
// The counterpart of `Fingerprint` in libs/go/observability, and deliberately the
// same RULE rather than the same code: a browser error and a service error that
// describe one fault should group together on one dashboard, and they cannot if
// the two sides normalise differently.
//
// It lives here rather than in the frontend because ADR-0503 requires the
// fingerprint to be centralised and unit-tested — and `mise run test:ts` runs
// `libs/ts/`, where a test is cheap to run and impossible to forget.

/** The character classes that vary between occurrences of one fault. */
const ALNUM = /^[0-9a-zA-Z]$/;
/** `at fn (…)` — the frame's name, when it has one. */
const NAMED_FRAME = /^at\s+([^\s(]+)\s*\(/;
/** The `at ` prefix and the `:line:col` suffix an anonymous frame carries. */
const AT_PREFIX = /^at\s+/;
const LOCATION_SUFFIX = /:\d+:\d+\)?$/;

/**
 * Removes the parts of a message that vary per occurrence, leaving the skeleton
 * the code wrote.
 *
 * Two rules, matching the Go side exactly:
 *
 * - a run of digits becomes `<n>` — counts, sizes, ports, status codes;
 * - a run of letters and digits together, four characters or longer, becomes
 *   `<x>` — entity ids, uuids, digests.
 *
 * Not a hex rule: the wire form of an identifier here is Crockford base32
 * (`order_01kztmx9e0fq1r13w5d1aerqw6`), whose alphabet includes letters no hex
 * run matches, so a hex rule shreds one id into fragments that still differ
 * between occurrences.
 */
export function normalise(message: string): string {
  let out = "";
  let i = 0;
  while (i < message.length) {
    const char = message[i] as string;
    if (ALNUM.test(char)) {
      i = consumeRun(message, i, (piece) => {
        out += piece;
      });
    } else {
      out += char;
      i += 1;
    }
  }
  return out;
}

/**
 * Consumes one alphanumeric run starting at `start`, appends its replacement, and
 * returns the index after it.
 */
function consumeRun(message: string, start: number, emit: (piece: string) => void): number {
  let j = start;
  let digits = 0;
  let letters = 0;
  while (j < message.length && ALNUM.test(message[j] as string)) {
    const c = message[j] as string;
    if (c >= "0" && c <= "9") {
      digits += 1;
    } else {
      letters += 1;
    }
    j += 1;
  }
  const run = message.slice(start, j);
  if (letters === 0) {
    emit("<n>");
  } else if (digits > 0 && run.length >= 4) {
    emit("<x>");
  } else {
    emit(run);
  }
  return j;
}

/**
 * The application frames of a stack trace, in order, with locations stripped.
 *
 * Line and column numbers are removed for the reason the Go side removes them:
 * editing a line above a function shifts every line below it, and a fingerprint
 * that moves on a cosmetic edit mints a new fault for a change that fixed
 * nothing.
 *
 * Vendor frames are dropped by the same argument the Go side uses — the frames
 * inside a framework are identical for every caller, so keeping them merges
 * unrelated faults that happen to fail in the same library. In a browser bundle
 * "vendor" is what came from `node_modules`, which the build records in the
 * source path.
 */
export function appFrames(stack: string | undefined, limit = 8): string[] {
  if (!stack) {
    return [];
  }
  return stack
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.startsWith("at ") && !isVendorFrame(line))
    .map(frameName)
    .slice(0, limit);
}

function isVendorFrame(line: string): boolean {
  return line.includes("node_modules") || line.includes("/_next/static/chunks/framework");
}

/**
 * The name of one stack frame, with its location stripped: `at fn (host/x.js:1:2)`
 * keeps `fn`. An anonymous frame keeps its path, minus the line and column.
 */
function frameName(line: string): string {
  const named = NAMED_FRAME.exec(line);
  if (named?.[1]) {
    return named[1];
  }
  return line.replace(AT_PREFIX, "").replace(LOCATION_SUFFIX, "");
}

/**
 * A stable identifier for the FAULT rather than for the occurrence.
 *
 * Hashes the error type, the normalised message, and the application frames —
 * the same three inputs, in the same order, as the Go side.
 *
 * The hash is FNV-1a rather than SHA-256, and that is a deliberate difference
 * from the service side. This runs in the browser on the error path, where
 * `crypto.subtle.digest` is asynchronous and would turn error reporting into a
 * promise chain; a non-cryptographic hash is the right tool because nothing here
 * depends on the hash being hard to invert. The two sides therefore produce
 * different values for the same fault — grouping is per surface, which is how the
 * dashboard reads them anyway.
 */
// biome-ignore-start lint/suspicious/noBitwiseOperators: FNV-1a is defined in terms of xor and a 32-bit multiply
export function fingerprint(error: { name?: string; message?: string; stack?: string }): string {
  const parts = [error.name ?? "Error", normalise(error.message ?? ""), ...appFrames(error.stack)];
  let hash = 0x81_1c_9d_c5;
  for (const part of parts) {
    for (let i = 0; i < part.length; i += 1) {
      hash ^= part.charCodeAt(i);
      // FNV prime, via shifts: `Math.imul` keeps the multiply in 32 bits, which
      // plain `*` does not once the value exceeds 2^53.
      hash = Math.imul(hash, 0x01_00_01_93) >>> 0;
    }
    // A separator between parts, so ["ab","c"] and ["a","bc"] do not collide.
    hash ^= 0;
    hash = Math.imul(hash, 0x01_00_01_93) >>> 0;
  }
  return hash.toString(16).padStart(8, "0");
}
// biome-ignore-end lint/suspicious/noBitwiseOperators: end of the hash
