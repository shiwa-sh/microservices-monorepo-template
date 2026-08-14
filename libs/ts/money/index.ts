/**
 * Monetary amounts on the TypeScript side (ADR-0300, ADR-0100).
 *
 * The counterpart of `libs/go/money`, and deliberately much smaller. Go owns the
 * arithmetic because Go owns the totals; a browser adds nothing to a price it was
 * handed. What a client does with money is READ it and DISPLAY it, so that is what
 * this exports:
 *
 *     parseMoney   validate what arrived, and refuse what did not
 *     formatMoney  render it for a human, in their locale
 *
 * The amount stays a string the whole way through. It arrives as a string because
 * `JSON.parse` turns a JSON number into an IEEE-754 double, and 12.10 is not
 * representable as one — a price rendered from a double is wrong in the last place
 * often enough to be noticed and rarely enough to be unattributable. Nothing here
 * converts it to a `number` except inside {@link formatMoney}, where the value is
 * already destined for a string and the fraction has at most four places.
 *
 * There is no arithmetic here on purpose. A client that needs to multiply a price
 * by a quantity is a client computing a total the server will compute again, and
 * the two answers will differ the first time a rounding rule or a tax applies. The
 * server sends the total.
 */

/** The wire form of a monetary amount: the shared `Money` component of every spec. */
export type Money = {
  /** Decimal, sign-prefixed when negative, no thousands separators. */
  readonly amount: string;
  /** ISO 4217 alphabetic code, uppercase. */
  readonly currency: string;
};

/** Thrown by {@link parseMoney}. Named so a caller can tell it from a network error. */
export class InvalidMoneyError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "InvalidMoneyError";
  }
}

/** The same pattern the specs enforce, so this refuses exactly what a 400 would. */
const AMOUNT_PATTERN = /^-?[0-9]+(\.[0-9]+)?$/;

const CURRENCY_PATTERN = /^[A-Z]{3}$/;

/**
 * The internal scale of `libs/go/money`, and the most decimal places an amount can
 * arrive with. It is finer than any currency's minor unit because a unit price can
 * be, and rejecting the excess is better than silently truncating it.
 */
const MAX_FRACTION_DIGITS = 4;

/**
 * parseMoney validates an untrusted value and returns it as {@link Money}.
 *
 * Use it at the edge — on a response body, on a form value — and not on a value
 * this module already returned. What it catches is a contract drift: a field that
 * became a number, a currency that arrived lowercase, a null where the spec said
 * required. Those reach a component as `undefined` and render as "NaN" or "$0.00",
 * which looks like a price rather than like a bug.
 */
export function parseMoney(value: unknown): Money {
  if (typeof value !== "object" || value === null) {
    throw new InvalidMoneyError(`money: expected an object, got ${typeof value}`);
  }
  const { amount, currency } = value as Record<string, unknown>;

  // A number here is the specific failure the string form exists to prevent, so it
  // gets its own message rather than "expected a string".
  if (typeof amount === "number") {
    throw new InvalidMoneyError(
      "money: amount is a JSON number, which cannot hold a decimal exactly",
    );
  }
  if (typeof amount !== "string" || !AMOUNT_PATTERN.test(amount)) {
    throw new InvalidMoneyError(`money: not a decimal amount: ${JSON.stringify(amount)}`);
  }
  const fraction = amount.split(".")[1] ?? "";
  if (fraction.length > MAX_FRACTION_DIGITS) {
    throw new InvalidMoneyError(
      `money: more than ${MAX_FRACTION_DIGITS} decimal places in ${amount}`,
    );
  }
  if (typeof currency !== "string" || !CURRENCY_PATTERN.test(currency)) {
    throw new InvalidMoneyError(
      `money: currency must be three uppercase letters: ${JSON.stringify(currency)}`,
    );
  }
  return { amount, currency };
}

/**
 * formatMoney renders an amount for a human: the currency's symbol, its own number
 * of minor digits, and the reader's grouping and decimal separators.
 *
 * The locale defaults to `undefined`, which is `Intl`'s "the runtime's locale". In
 * a Server Component that is the SERVER's locale, which is not the reader's — pass
 * the negotiated one explicitly on any surface where it matters. It is a parameter
 * rather than a module-level default because the correct value is per-request.
 *
 * `Intl.NumberFormat` decides the minor digits from the currency: two for EUR, zero
 * for JPY, three for KWD. Hardcoding two is the bug this call exists to avoid — it
 * renders ¥1000 as ¥10.00.
 */
export function formatMoney(value: Money, locale?: string): string {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: value.currency,
    // A unit price can carry four decimal places while the currency has two. Letting
    // the maximum float up to the amount's own precision shows the price that was
    // charged rather than a rounded one, without padding whole amounts with zeros
    // the currency does not use.
    maximumFractionDigits: Math.max(
      fractionDigits(value.amount),
      currencyDigits(value.currency, locale),
    ),
  }).format(Number(value.amount));
}

/** The decimal places actually present in the amount. */
function fractionDigits(amount: string): number {
  return (amount.split(".")[1] ?? "").length;
}

/** The currency's own minor-unit digits, as the runtime's CLDR data has them. */
function currencyDigits(currency: string, locale?: string): number {
  return (
    new Intl.NumberFormat(locale, { style: "currency", currency }).resolvedOptions()
      .maximumFractionDigits ?? 2
  );
}
