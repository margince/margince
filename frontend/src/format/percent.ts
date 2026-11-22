import type { Locale } from "../i18n";
import { numberFormat } from "./formatters";

/**
 * A share of a whole as a whole-number percentage: "42%", or "42 %" for a
 * German reader, whose typography puts a space before the sign.
 *
 * Whole numbers only, because what this labels is progress (a ring, a chip).
 * There "41.7%" claims a precision a count of scanned messages does not have.
 * A rate that carries a real fraction is `formatFinePercent`'s job.
 */
export function formatPercent(
  fraction: number,
  locale: Locale,
  fractionDigits = 0,
): string {
  return numberFormat(locale, {
    style: "percent",
    maximumFractionDigits: fractionDigits,
  }).format(fraction);
}

/**
 * A share of a whole where the FRACTION matters: "0.4 %", "12.0 %".
 *
 * `formatPercent` rounds to whole numbers, which is right for progress. It is
 * wrong for a rate held to a threshold below one percent. A transition undone
 * at 0.4% would print "0%" and read as flawless, where the rounding misleads.
 *
 * Zero is still "0 %": nothing went wrong is a whole fact, and "0.0 %" claims a
 * measurement where there is simply nothing to measure.
 */
export function formatFinePercent(fraction: number, locale: Locale): string {
  if (fraction === 0) {
    return formatPercent(0, locale);
  }
  return numberFormat(locale, {
    style: "percent",
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(fraction);
}
