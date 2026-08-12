/**
 * Entity identifiers on the TypeScript side (ADR-0003).
 *
 * The counterpart of `libs/go/id`: a UUIDv7 (RFC 9562) carried on the wire in the
 * TypeID form — a type prefix, an underscore, and the UUID in 26 characters of
 * Crockford base32.
 *
 *     order_01j8xk7m3q0000000000000000
 *     {prefix}_{UUIDv7 in 26 characters of base32}
 *
 * The two implementations are checked against the same published TypeID vectors
 * rather than against each other, so neither can drift into a private encoding that
 * only its own round-trip test accepts.
 *
 * A consumer treats an identifier as opaque (AIP-122): nothing here orders or
 * constructs one from parts. Parsing exists to VALIDATE what arrived and to name
 * its type — a client that has to build an identifier is a client the API failed.
 */

/** Crockford base32 in TypeID's ordering: no i, l, o, or u. */
const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

/** 128 bits at 5 bits per character needs 25.6 characters; the first carries the 3 spare bits. */
const ENCODED_LENGTH = 26;

/** The same 63-character bound a DNS label imposes, which is the tightest downstream. */
const MAX_PREFIX_LENGTH = 63;

/** Hoisted so the pattern is compiled once rather than per call. */
const PREFIX_PATTERN = /^[a-z_]+$/;
const HEX_UUID_PATTERN = /^[0-9a-f]{32}$/;
const HYPHEN_PATTERN = /-/g;

const DECODE = new Map<string, number>([...ALPHABET].map((character, value) => [character, value]));

/** A parsed identifier: its type and the underlying UUID in canonical hyphenated form. */
export type ParsedId = {
  readonly prefix: string;
  /** Canonical 8-4-4-4-12 lowercase hex, the form a `uuid` column holds. */
  readonly uuid: string;
};

export class InvalidIdError extends Error {
  constructor(message: string) {
    super(`invalid identifier: ${message}`);
    this.name = "InvalidIdError";
  }
}

/** Reports whether a prefix is well-formed: lowercase a–z and underscores, not at either end. */
export function isValidPrefix(prefix: string): boolean {
  if (prefix.length === 0 || prefix.length > MAX_PREFIX_LENGTH) {
    return false;
  }
  // The LAST underscore separates prefix from suffix, so a trailing one would make
  // the prefix ambiguous.
  if (prefix.startsWith("_") || prefix.endsWith("_")) {
    return false;
  }
  return PREFIX_PATTERN.test(prefix);
}

/**
 * Parse a wire identifier, requiring the expected type.
 *
 * Passing `expectedPrefix` is the point of the whole convention: it is what stops an
 * `order_` reaching a function that wanted a `product_`. Omit it only where the type
 * genuinely is not known ahead of time.
 */
export function parseId(value: string, expectedPrefix?: string): ParsedId {
  const separator = value.lastIndexOf("_");
  if (separator < 0) {
    throw new InvalidIdError("no prefix separator");
  }

  const prefix = value.slice(0, separator);
  const suffix = value.slice(separator + 1);

  if (!isValidPrefix(prefix)) {
    throw new InvalidIdError(`bad prefix ${JSON.stringify(prefix)}`);
  }
  if (expectedPrefix !== undefined && prefix !== expectedPrefix) {
    throw new InvalidIdError(`want ${expectedPrefix}, got ${prefix}`);
  }

  return { prefix, uuid: decodeSuffix(suffix) };
}

/** Reports whether a value is a well-formed identifier, optionally of a given type. */
export function isId(value: string, expectedPrefix?: string): boolean {
  try {
    parseId(value, expectedPrefix);
    return true;
  } catch {
    return false;
  }
}

/** The type a wire identifier names, or null when it is not one. */
export function prefixOf(value: string): string | null {
  try {
    return parseId(value).prefix;
  } catch {
    return null;
  }
}

/**
 * Render a UUID in the wire form. Present for the rare boundary that holds a bare
 * UUID — a log processor, a migration script — not for application code, which
 * receives identifiers already encoded.
 */
export function formatId(prefix: string, uuid: string): string {
  if (!isValidPrefix(prefix)) {
    throw new InvalidIdError(`bad prefix ${JSON.stringify(prefix)}`);
  }
  return `${prefix}_${encodeUuid(uuid)}`;
}

/** Decode 26 base32 characters into a canonical hyphenated UUID. */
function decodeSuffix(suffix: string): string {
  if (suffix.length !== ENCODED_LENGTH) {
    throw new InvalidIdError(`suffix must be ${ENCODED_LENGTH} characters`);
  }

  const first = DECODE.get(suffix[0] ?? "");
  if (first === undefined) {
    throw new InvalidIdError("suffix is not Crockford base32");
  }
  // 26 characters hold 130 bits and a UUID is 128, so a leading value above 7 does
  // not fit — rejecting it is what stops a silent wrap.
  if (first > 7) {
    throw new InvalidIdError("suffix overflows 128 bits");
  }

  const bytes = new Uint8Array(16);
  bytes[0] = first << 5;

  let bit = 3;
  for (let i = 1; i < ENCODED_LENGTH; i++) {
    const value = DECODE.get(suffix[i] ?? "");
    if (value === undefined) {
      throw new InvalidIdError("suffix is not Crockford base32");
    }
    for (let shift = 4; shift >= 0; shift--) {
      const byteIndex = bit >> 3;
      const bitIndex = bit & 7;
      bytes[byteIndex] |= ((value >> shift) & 1) << (7 - bitIndex);
      bit++;
    }
  }
  return toHyphenated(bytes);
}

function encodeUuid(uuid: string): string {
  const hex = uuid.replace(HYPHEN_PATTERN, "").toLowerCase();
  if (!HEX_UUID_PATTERN.test(hex)) {
    throw new InvalidIdError("not a UUID");
  }

  const bytes = new Uint8Array(16);
  for (let i = 0; i < 16; i++) {
    bytes[i] = Number.parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }

  const out: string[] = [ALPHABET[((bytes[0] ?? 0) & 0xe0) >> 5] ?? "0"];
  let bit = 3;
  for (let i = 1; i < ENCODED_LENGTH; i++) {
    let value = 0;
    for (let n = 0; n < 5; n++) {
      const byteIndex = bit >> 3;
      const bitIndex = bit & 7;
      value = (value << 1) | (((bytes[byteIndex] ?? 0) >> (7 - bitIndex)) & 1);
      bit++;
    }
    out.push(ALPHABET[value] ?? "0");
  }
  return out.join("");
}

function toHyphenated(bytes: Uint8Array): string {
  const hex = [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
  return [
    hex.slice(0, 8),
    hex.slice(8, 12),
    hex.slice(12, 16),
    hex.slice(16, 20),
    hex.slice(20, 32),
  ].join("-");
}
