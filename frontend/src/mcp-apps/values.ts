// How a view renders a value it did not produce: a missing or malformed one
// reads as the em dash, never as "NaN", "undefined" or "0".

import { minorUnitDigits, toMajorUnits } from "../format/minorunits";
import { asFiniteNumber, asText } from "./types";

/** What a view shows for a value it does not have. Never "NaN", never "0". */
export const ABSENT = "—";

/** percent renders a proportion, or the em dash for a value it cannot read. */
export function percent(value: unknown): string {
  const n = asFiniteNumber(value);
  return n === null ? ABSENT : `${Math.round(n * 100)}%`;
}

/** count renders a whole number, or the em dash for a value it cannot read. */
export function count(value: unknown): string {
  const n = asFiniteNumber(value);
  return n === null ? ABSENT : String(n);
}

/**
 * money renders integer minor units scaled by the currency's own minor-unit
 * count, never by a fixed 100. The count is format/minorunits', which mirrors
 * the server's ISO 4217 table; Intl follows CLDR, and the two disagree on ten
 * codes. minorunits takes no locale, so no translation code follows it into a
 * document inlined into a third-party host.
 *
 * The locale is the host browser's. No LocaleProvider stands above this
 * document and no reader's choice reaches it; `format/one-locale.test.ts`
 * names this file as an exemption for that reason.
 *
 * An unknown currency or an amount past the safe integer range renders as the
 * em dash. Intl throws on the first, and the second was already rounded when
 * the wire's int64 became a double.
 */
export function money(amountMinor: unknown, currency: unknown): string {
  const minor = asFiniteNumber(amountMinor);
  const code = asText(currency);
  if (minor === null || code === "" || !Number.isSafeInteger(minor))
    return ABSENT;
  try {
    const digits = minorUnitDigits(code);
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: code,
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(toMajorUnits(minor, code));
  } catch {
    return ABSENT;
  }
}

/**
 * day renders an instant as its calendar day in UTC, the zone the server judged
 * "overdue" in. A day in the reader's zone could print tomorrow's date beside it.
 */
export function day(value: unknown): string {
  const text = asText(value);
  if (text === "") return ABSENT;
  const at = new Date(text);
  if (Number.isNaN(at.getTime())) return ABSENT;
  return at.toISOString().slice(0, 10);
}

/**
 * compactMoney renders an amount the way a headline states it: "€115K". The
 * scale follows the currency's own minor units, as `money` does.
 */
export function compactMoney(amountMinor: unknown, currency: unknown): string {
  const minor = asFiniteNumber(amountMinor);
  const code = asText(currency);
  if (minor === null || code === "" || !Number.isSafeInteger(minor))
    return ABSENT;
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: code,
      notation: "compact",
      maximumSignificantDigits: 3,
    }).format(toMajorUnits(minor, code));
  } catch {
    return ABSENT;
  }
}
