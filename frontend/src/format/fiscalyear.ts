// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * How a period bucket reads for a given fiscal start — the SAME rule the server
 * cuts reports with, spelled here so an admin can see the consequence of the
 * setting before they save it.
 *
 * Two spellings of one rule, and this comment is where that is admitted. The
 * authority is `fiscalYearLabel` in the backend's
 * internal/compose/reportperiod.go, which builds the SQL a report is actually
 * cut by: the server's label is what a report carries, what a saved view
 * filters on, and what travels in a derivation handle. Nothing here reaches any
 * of that — this module only PREVIEWS the shape on the settings screen, so a
 * drift between the two misleads an admin for one screen rather than
 * mislabelling a report.
 *
 * That is why the preview is not derived from the server: asking it would mean
 * running a report to render a settings row. The mirror is held instead by
 * `backend/frontendfiscalyear_test.go`, which executes both spellings against
 * the same months and fails in either direction — the pattern
 * `values.MinorUnitExceptions()` is held against `format/minorunits.ts` by.
 */

/**
 * The label a fiscal year starting in `startMonth` carries, for a year that
 * begins in `startYear`.
 *
 * A January start is the calendar year and is spelled as one — `2026`, not
 * `FY2026/27`, which would be false: it does not span 2027. Every other start
 * spans two calendar years and names both, because "FY2026" alone means the
 * starting year to a British reader and the ending year to an Australian one,
 * and this product is deployed in Europe and Vietnam at once.
 */
export function fiscalYearLabel(startMonth: number, startYear: number): string {
  if (startMonth === 1) {
    return String(startYear);
  }
  // The last two digits of the following year. Derived by adding rather than
  // by slicing the string, so the century rolls over on its own: a year
  // beginning in 2099 ends in 2100 and reads "FY2099/00".
  const ends = String((startYear + 1) % 100).padStart(2, "0");
  return `FY${startYear}/${ends}`;
}
