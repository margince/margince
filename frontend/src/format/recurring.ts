/**
 * The monthly reading of an annual recurring figure.
 *
 * The twin of `values.MonthlyEquivalent` in the Go kernel, and the two are
 * held together by one corpus rather than by inspection: both this module's
 * test and the Go one read
 * `backend/internal/shared/kernel/values/testdata/monthly_equivalent.json`, so
 * a case answered differently on either side fails there.
 *
 * The rounding is HALF UP on the minor unit, because an arbitrary annual
 * figure does not divide into twelve and the product still has to print
 * something. The result is approximate whenever the division lost anything,
 * and that travels with it: twelve of an approximate monthly figure do not add
 * back to the year, and a surface printing it as exact invites somebody to
 * multiply it.
 */

/** The annual figure divided into months, with whether anything was lost. */
export type MonthlyReading = Readonly<{
  monthlyMinor: number;
  approximate: boolean;
}>;

const MONTHS_PER_YEAR = 12;

/**
 * Divides an annual recurring figure into a monthly one, in the same minor
 * units.
 *
 * Never reads the currency: the minor-unit scale is already applied on both
 * sides of the division, so a currency with no minor unit divides exactly as
 * one with three does.
 *
 * A figure this cannot divide exactly returns null rather than throwing. The
 * server stores ARR as a 64-bit integer and JSON hands it here as a double, so
 * a figure past Number.MAX_SAFE_INTEGER arrives having already lost precision —
 * real, legal, and not something this can read. Throwing would take the whole
 * record page down over a derived line beside a figure that displays fine on
 * its own; the callers render the annual figure and omit the monthly one.
 */
export function monthlyEquivalent(annualMinor: number): MonthlyReading | null {
  if (!Number.isSafeInteger(annualMinor)) {
    return null;
  }
  const quotient = Math.trunc(annualMinor / MONTHS_PER_YEAR);
  const remainder = annualMinor % MONTHS_PER_YEAR;
  if (remainder === 0) {
    return { monthlyMinor: quotient, approximate: false };
  }
  // Half up: a remainder of six or more carries. Doubling rather than
  // comparing against six keeps the test exact and matches the Go side line
  // for line, which is what makes the two readable against each other.
  const carries = Math.abs(remainder) * 2 >= MONTHS_PER_YEAR;
  const adjusted = carries ? quotient + (remainder < 0 ? -1 : 1) : quotient;
  return { monthlyMinor: adjusted, approximate: true };
}
