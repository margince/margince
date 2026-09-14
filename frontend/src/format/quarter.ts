// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Which quarter a date-only wire value falls in, as the label a reading wears.
 *
 * Read off the MONTH IN THE STRING rather than through `Date`. A forecast
 * period arrives date-only ("2026-07-01"): there is no instant in it to
 * localize, and parsing one into a `Date` pins it to midnight UTC, which reads
 * as the day before west of the meridian and labels a July quarter `Q2`.
 *
 * `Q3` is the label in all three catalogs — the letter and the numeral are what
 * German and Vietnamese sales write too — so this is a format rather than a
 * message, the same standing `fiscalYearLabel` has beside it.
 *
 * The CALENDAR quarter, deliberately, and not the installation's fiscal one.
 * The figure it labels is a window the server already chose and reports by its
 * start and end dates; naming that window after a fiscal year the client holds
 * no setting for would label the server's period with somebody else's calendar.
 */
export function quarterLabel(dateOnly: string): string | null {
  const month = Number(dateOnly.slice(5, 7));
  // Null rather than a guess, so a caller names the reading without a quarter
  // instead of printing one nobody can reconcile: a period whose start the page
  // cannot read is not a period.
  if (!Number.isInteger(month) || month < 1 || month > 12) {
    return null;
  }
  return `Q${Math.ceil(month / 3)}`;
}
