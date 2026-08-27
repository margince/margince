import { formatMoneyOrAbsent } from "../format/format";
import type { Locale } from "../i18n";

// A history value read the way the rest of the record page reads it.
//
// The audit spine projects every value as a string, including the integer
// minor-unit count money is stored in. Rendered raw, a deal moving from
// 2500000 to 4150000 reads as millions beside tiles that show the same figure
// as €25,000.00 — and a value the reader misreads by a hundredfold is the one
// thing that must not sit next to a button that puts changes back.
//
// The scale is the CURRENCY's and never a constant: `formatMoneyOrAbsent`
// resolves it through `format/minorunits.ts`, which mirrors the server's ISO
// 4217 table, so a currency with no minor unit keeps its digits instead of
// losing two of them.

// A column holding an integer minor-unit amount, by the suffix the contract
// spells it with. A rule rather than a list: a money column added upstream is
// formatted the day it appears, where a list would render the new one raw.
export function isMinorUnitField(field: string): boolean {
  return field.endsWith("_minor");
}

// The rendered value, or null for a value the record does not hold — the diff
// draws its own "created" and "cleared" wording for that and must keep being
// able to tell the two apart from a value that happens to be empty text.
export function historyValue(
  field: string,
  value: string | null | undefined,
  currency: string | null | undefined,
  locale: Locale,
): string | null {
  if (value === null || value === undefined) {
    return null;
  }
  if (!isMinorUnitField(field)) {
    return value;
  }
  const minor = Number(value);
  // A minor-unit column whose value is not an integer is not an amount this
  // function can scale, and inventing a reading for it would be a worse answer
  // than the one the spine recorded.
  if (!Number.isInteger(minor)) {
    return value;
  }
  return formatMoneyOrAbsent(minor, currency, locale);
}
