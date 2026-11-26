// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  ambiguousReadings,
  decimalSign,
  parseTypedDecimal,
  writtenDecimal,
} from "./decimalinput";

// The scale between an amount as a human types it and the integer this
// product stores.
//
// It is NOT presentation, and that is why it is its own module beside the
// formatters rather than inside them: the same table governs the write
// direction, where locale must never reach, and format.ts's own header says
// locale never flows back into storage. These functions take no locale
// deliberately — a currency's minor-unit count is a property of the currency,
// not of who is reading. typedMinorUnits is the one exception, and its locale
// only refuses an ambiguous text; it never changes the number stored.
//
// It exists because the write direction had no owner at all. Eleven call sites
// spelled `Math.round(Number(amount) * 100)` and `valueMinor / 100`, which is
// right for the euro and wrong for every currency that is not two-decimal: VND,
// JPY and KRW have no minor unit, so a dong price typed as 18,000,000 was
// stored as 1,800,000,000, and a Kuwaiti dinar — three digits — was stored at a
// tenth.
//
// WHY NOT Intl. The first version of this module read the count from
// `Intl.NumberFormat(...).resolvedOptions().maximumFractionDigits`, on the
// grounds that the runtime already ships the answer and maintains it. It ships
// a DIFFERENT answer: Intl follows CLDR, which records how a currency is used,
// and the server follows ISO 4217, which records what the standard assigns.
// They disagree on ten codes, and two of them are ordinary spendable money —
// CLDR gives MGA and IRR zero digits where ISO gives two, and IQD zero where
// ISO gives three. A browser scaling by CLDR and a server scaling by ISO is the
// hundredfold disagreement this whole change exists to remove, reintroduced one
// currency over.
//
// So the table below MIRRORS shared/kernel/values/minorunits.go, exactly, and
// TestTheFrontendMinorUnitTableMatchesTheGoOne (backend/gates/frontendminorunits_test.go)
// fails when the two drift apart in either direction. Intl is still the right
// tool for DISPLAY — grouping, symbol placement, the reader's own conventions —
// and format.ts uses it for that. It is the wrong tool for deciding an integer
// two systems must agree on.

// MINOR_UNIT_EXCEPTIONS lists the ISO 4217 codes whose minor unit is not the
// usual two digits. Most currencies, including EUR and USD, carry two, so this
// names the departures and the default below carries the rest.
//
// A code missing here renders at two digits and is wrong for that code — which
// is the tolerable failure, because refusing an unnamed code would suppress the
// amount for nearly every currency on earth while admitting the two dozen
// exceptions. Adding a code is a one-line change HERE AND IN THE GO TABLE; the
// gate refuses one without the other.
const MINOR_UNIT_EXCEPTIONS: Readonly<Record<string, number>> = {
  BIF: 0,
  CLP: 0,
  DJF: 0,
  GNF: 0,
  ISK: 0,
  JPY: 0,
  KMF: 0,
  KRW: 0,
  PYG: 0,
  RWF: 0,
  UGX: 0,
  UYI: 0,
  VND: 0,
  VUV: 0,
  XAF: 0,
  XAG: 0,
  XAU: 0,
  XDR: 0,
  XOF: 0,
  XPD: 0,
  XPF: 0,
  XPT: 0,
  XTS: 0,
  XXX: 0,
  BHD: 3,
  IQD: 3,
  JOD: 3,
  KWD: 3,
  LYD: 3,
  OMR: 3,
  TND: 3,
  CLF: 4,
  UYW: 4,
};

// minorUnitDigits reports how many minor-unit digits a currency carries.
//
// Two is ISO 4217's own default and what an unknown or malformed code answers,
// rather than throwing: a currency we cannot place is still one somebody typed
// an amount into, and refusing to scale it would store the raw digits as minor
// units.
export function minorUnitDigits(currency: string): number {
  return MINOR_UNIT_EXCEPTIONS[currency.trim().toUpperCase()] ?? 2;
}

// toMinorUnits converts a major-unit amount — what the human typed — into the
// integer the API stores, or NaN when it cannot do so exactly.
//
// It REFUSES a figure finer than its currency rather than rounding one. That is
// the rule documentextraction already stated for the same reason: "a figure
// with more decimals than the currency has is a misread, and silently dropping
// a digit is how an amount becomes wrong by an order of magnitude." A tenth of
// a cent is not a rounding question, it is a number somebody got wrong.
//
// Refusing is also what removes the double-rounding this function had. Rounding
// to a working precision and then rounding again gave answers that were wrong
// rather than merely approximate: 1.004951 EUR came out 101 cents where 100.4951
// rounds to 100, and 0.004951 came out a whole cent for less than half of one.
// With over-precise input refused there is one rounding left, and it only ever
// resolves a binary artefact.
//
// The scaling shifts a decimal STRING rather than multiplying a float, because
// `1.005 * 100` is 100.49999999999999 and a multiply would lose the cent on an
// amount stated exactly. Shifting the point in the text cannot.
//
// Halves round AWAY from zero, so a credit and a charge of the same size scale
// to the same magnitude — Math.round sends -0.5 to -0 and 0.5 to 1.
//
// NaN, rather than 0, is the refusal: a caller building a request body writes
// `amount_minor: 0` for a garbage input, and zero is a legal price. NaN
// serialises to null, which the nullable money fields take as unpriced and the
// non-nullable ones refuse — the API decides, not a silent default.
export function toMinorUnits(major: number, currency: string): number {
  if (!Number.isFinite(major)) {
    return Number.NaN;
  }
  const digits = minorUnitDigits(currency);
  // Finer than this currency can hold. Compared against the value's own
  // rounding rather than against the typed text, because the text is already
  // gone by the time a number arrives here.
  if (Number(major.toFixed(digits)) !== major) {
    return Number.NaN;
  }
  if (digits === 0) {
    return Number.isSafeInteger(major) ? major : Number.NaN;
  }
  // Enough places that the shift below never has to round a binary artefact
  // back into position; the value is already known to need no more than
  // `digits` of them.
  const text = major.toFixed(digits + 2);
  const negative = text.startsWith("-");
  const [whole, frac = ""] = (negative ? text.slice(1) : text).split(".");
  const shifted = Number(
    `${whole}${frac.slice(0, digits)}.${frac.slice(digits)}`,
  );
  const scaled = roundHalfAwayFromZero(shifted);
  const minor = negative ? -scaled : scaled;
  return Number.isSafeInteger(minor) ? minor : Number.NaN;
}

function roundHalfAwayFromZero(value: number): number {
  return value < 0 ? -Math.round(-value) : Math.round(value);
}

// toMajorUnits is the inverse, for seeding an input from a stored amount.
//
// It returns a number and not a string on purpose: the caller decides how many
// digits to show, and a currency with no minor unit must not be handed a ".00"
// that says it has one.
export function toMajorUnits(amountMinor: number, currency: string): number {
  return amountMinor / 10 ** minorUnitDigits(currency);
}

/** Why typed text is not an amount, as the message the field shows. */
export type AmountRefusal = Readonly<{
  key: MessageKey;
  params: Record<string, string>;
}>;

export type TypedMinor =
  | Readonly<{ minor: number }>
  | Readonly<{ refused: AmountRefusal }>;

/**
 * The one converter from an amount the reader typed to the minor units the
 * API stores. Every money box reads its text here, so a typed "12,5" means
 * the same amount in every form, and "1.500" is refused in every form.
 */
export function typedMinorUnits(
  text: string,
  currency: string,
  locale: Locale,
): TypedMinor {
  // A blank code would scale at the two-digit default, and the amount would
  // move a hundredfold once the real currency is known.
  if (currency.trim() === "") {
    return { refused: { key: "amount.refused.noCurrency", params: {} } };
  }
  const digits = minorUnitDigits(currency);
  const parsed = parseTypedDecimal(text, locale);
  if ("refused" in parsed) {
    return { refused: unreadAmount(parsed.refused, text, digits, locale) };
  }
  const minor = toMinorUnits(parsed.value, currency);
  if (!Number.isNaN(minor)) {
    return { minor };
  }
  // toMinorUnits refuses for precision or for size; precision is the one
  // whose rounding moves the value.
  if (Number(parsed.value.toFixed(digits)) !== parsed.value) {
    return {
      refused: { key: "record.amountTooPrecise", params: { currency } },
    };
  }
  return { refused: { key: "amount.refused.tooLarge", params: {} } };
}

/**
 * The minor units of a text a field's validate has already accepted, or NaN.
 * For a request mapper: NaN serialises to null, and the API decides.
 */
export function typedMinorOrNaN(
  text: string,
  currency: string,
  locale: Locale,
): number {
  const typed = typedMinorUnits(text, currency, locale);
  return "minor" in typed ? typed.minor : Number.NaN;
}

/** The message under a money field, or undefined for an amount or no text. */
export function typedAmountFault(
  text: string,
  currency: string,
  locale: Locale,
  t: Translator,
): string | undefined {
  if (text.trim() === "") {
    return undefined;
  }
  const typed = typedMinorUnits(text, currency, locale);
  return "refused" in typed
    ? t(typed.refused.key, typed.refused.params)
    : undefined;
}

/**
 * A stored amount as the text a money box opens on. Written with the reader's
 * decimal sign: a dinar's "1.234" in German would read as grouped and be
 * refused when saved back unchanged.
 */
export function typedAmountText(
  amountMinor: number,
  currency: string,
  locale: Locale,
): string {
  const digits = minorUnitDigits(currency);
  return writtenDecimal(
    toMajorUnits(amountMinor, currency).toFixed(digits),
    locale,
  );
}

function unreadAmount(
  refusal: "ambiguousGrouping" | "grouping" | "notNumber",
  text: string,
  digits: number,
  locale: Locale,
): AmountRefusal {
  const sign = decimalSign(locale);
  if (refusal === "ambiguousGrouping") {
    const { whole } = ambiguousReadings(text, locale);
    return digits === 0
      ? { key: "amount.refused.ambiguousWhole", params: { whole } }
      : {
          key: "amount.refused.ambiguous",
          params: { whole, decimal: `${whole}${sign}${"0".repeat(digits)}` },
        };
  }
  const example =
    digits === 0 ? "1234" : `1234${sign}${"5678".slice(0, digits)}`;
  return {
    key:
      refusal === "grouping"
        ? "amount.refused.grouping"
        : "amount.refused.notNumber",
    params: { example },
  };
}

/**
 * A running total of minor units, or undefined once it has passed 2^53. Each
 * amount the server sends is bounded; a sum of many is not, and past 2^53 it
 * silently becomes another number. A caller shows the undefined total as too
 * large rather than as a figure.
 */
export function addMinor(
  total: number | undefined,
  amount: number,
): number | undefined {
  if (total === undefined) return undefined;
  const sum = total + amount;
  return Number.isSafeInteger(sum) ? sum : undefined;
}

/** The exact sum of amounts, or undefined once it passes the exact range. */
export function sumMinor(amounts: readonly number[]): number | undefined {
  return amounts.reduce<number | undefined>(addMinor, 0);
}
