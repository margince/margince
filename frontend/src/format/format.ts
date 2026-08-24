import type { Locale } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { minorUnitDigits, toMajorUnits } from "./minorunits";

// The presentation edge (architecture/10 §1–3): everything here formats
// ALREADY-stored values — minor units, UTC instants, IR-provided base
// aggregates. No FX math, no live rate calls, no calendar arithmetic, and
// locale never flows back into storage. format.test.ts pins each rule.

// Exported because a relative-time formatter needs the same mapping, and two
// tables from one locale set is how a page ends up formatting one date German
// and the one under it English.
export const INTL_LOCALE: Record<Locale, string> = {
  de: "de-DE",
  en: "en-GB", // A100: unconfigured English is en-GB, not en-US
  vi: "vi-VN",
};

// Money arrives as integer minor units + ISO currency (data-semantics §1).
// The only transformation is the currency's minor-unit scaling — display,
// not arithmetic.
//
// The SCALE comes from format/minorunits, which mirrors the server's ISO 4217
// table; only the FORMATTING is Intl's. Those are different questions and Intl
// answers the second one better: it knows where the symbol goes and how a
// reader groups digits. It answers the first one differently, because CLDR
// records how a currency is used rather than what ISO assigns — they disagree
// on ten codes, and dividing by Intl's count while the server multiplied by
// ISO's would show a stored IQD 1234 as 1234 here and as 1.234 on the server.
// The reader would be looking at a number the record does not hold.
export function formatMoney(
  amountMinor: number,
  currency: string,
  locale: Locale,
): string {
  const digits = minorUnitDigits(currency);
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "currency",
    currency,
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(toMajorUnits(amountMinor, currency));
}

// What the product shows for a money figure it does not have. Not a zero and
// not a guessed currency: "€0 open" and "we do not know" are different claims,
// and a euro sign on a figure that might be dong is worse than no figure.
export const MONEY_ABSENT = "—";

/**
 * A money reading, or nothing. BOTH halves are required.
 *
 * Money on the wire is an integer minor amount PLUS its ISO currency
 * (data-semantics §1), and either half can be absent: the amount is null when
 * the server could not compute one, the currency is null on a deal nobody has
 * priced. Neither absence has a safe default. A missing currency rendered as
 * EUR puts a euro sign on a figure that might be dollars, and the reader
 * cannot tell it apart from a real euro amount; a missing amount rendered as 0
 * states a figure the server never sent.
 *
 * This is the one spelling of that rule. It was written independently in six
 * screens before it lived here, and the sites that did NOT write it defaulted
 * to EUR instead — including one that reached Intl with an empty currency code
 * and threw mid-render.
 */
export function formatMoneyOrAbsent(
  amountMinor: number | null | undefined,
  currency: string | null | undefined,
  locale: Locale,
): string {
  if (amountMinor == null || !currency) {
    return MONEY_ABSENT;
  }
  return formatMoney(amountMinor, currency, locale);
}

/**
 * A money figure for a KPI SLOT: €428k rather than €428,000.00.
 *
 * The strip gives six readings roughly 110px of usable width each, and a full
 * euro amount does not fit — it wraps mid-number or clips, and "€201,099.0" is
 * a different number rather than a smaller rendering of the right one. Both
 * mockups abbreviate here for the same reason.
 *
 * Only where the exact figure is not the point. The finance card below the
 * strip renders `formatMoney` in full, and it is one scroll away — this is the
 * scale of the account, that is the amount.
 *
 * Under 10,000 it stays exact: "€8,332" is as short as "€8k" and says more.
 */
export function formatMoneyCompact(
  amountMinor: number,
  currency: string,
  locale: Locale,
): string {
  // Our scale, Intl's formatting — see formatMoney for why the two are
  // deliberately not the same source.
  const major = toMajorUnits(amountMinor, currency);
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "currency",
    currency,
    // `compact` only kicks in at 1000; below 10_000 the long form is no wider
    // and carries every digit, so the threshold is where abbreviating buys
    // something.
    notation: Math.abs(major) >= 10_000 ? "compact" : "standard",
    maximumFractionDigits: Math.abs(major) >= 10_000 ? 1 : 0,
  }).format(major);
}

export function formatNumber(value: number, locale: Locale): string {
  return new Intl.NumberFormat(INTL_LOCALE[locale]).format(value);
}

// IANA zone names only (AC-DS-TZ4): fixed offsets ("+01:00", "Etc/GMT-1")
// silently freeze DST rules — reject them loudly at the edge.
function assertIanaZone(zone: string): void {
  if (/^[+-]\d{2}:?\d{2}$/.test(zone) || /^(Etc\/)?GMT[+-]?\d*$/i.test(zone)) {
    throw new Error(
      `timezone must be an IANA name, got fixed offset "${zone}"`,
    );
  }
  // Intl itself rejects unknown names — constructing with the zone throws a
  // RangeError we let propagate; format() forces the value to be consumed.
  new Intl.DateTimeFormat("en-US", { timeZone: zone }).format();
}

/**
 * Whether the formatters above will accept this zone, asked without throwing.
 *
 * A page rendering a LIST of moments cannot let one row's zone take the page
 * down, so it needs to ask the question before it formats. Asking it any other
 * way is how the two answers come apart: a caller that probed with Intl alone
 * learned only that the name resolves, which every fixed offset does — so
 * `Etc/GMT-1`, `GMT` and `+01:00` all passed the probe and then threw inside
 * `formatDate`, in the exact place the probe existed to protect.
 *
 * So the predicate is this module's, derived from the assertion rather than
 * restated beside it. There is no second reading of "a zone this renders".
 */
export function isRenderableZone(zone: string): boolean {
  try {
    assertIanaZone(zone);
    return true;
  } catch {
    return false;
  }
}

// Zone-by-purpose (architecture/10 §2): personal deadlines localize to the
// USER zone, reporting-period labels bucket on the WORKSPACE zone — the
// caller picks the purpose, this helper only attaches the zone.
export function formatDate(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(new Date(utcIso));
}

// A date a reader SCANS rather than keys into a form — a record's header
// meta line, never a table column someone might copy into a spreadsheet.
// The abbreviated month (Intl's own locale name, so de/vi read correctly
// with no lookup table of our own) is what tells the two apart from
// `formatDate` above; callers that need the numeric, sortable form keep
// using that one.
export function formatDateAbbrev(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(new Date(utcIso));
}

/**
 * A day and its month, with NO year — "21 Aug".
 *
 * For a date the surrounding text already places in time: a row inside a
 * period the reader picked, an employment span whose years are printed beside
 * it, a meeting the page has already called the next one. `formatDateAbbrev`
 * is the same rendering WITH the year and is the right one everywhere the
 * reader cannot tell which year is meant from the context.
 *
 * Four screens carried a byte-identical private copy of this before it lived
 * here, each with its own `undefined` locale — so the same person record
 * printed its dates in the browser's guessed locale on four surfaces and in
 * the reader's chosen one nowhere.
 */
export function formatDayMonth(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "numeric",
    month: "short",
  }).format(new Date(utcIso));
}

/**
 * The wall-clock time of an instant, with no date — "09:05".
 *
 * For a row whose date is already printed beside it, where repeating the date
 * per line is noise. One caller today: it lives here because this module is
 * where a locale reaches a formatter, not because a second caller is expected.
 * A private copy in the screen would be a second locale decision, which is the
 * thing this module exists to prevent.
 */
export function formatTimeOfDay(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(utcIso));
}

export function formatDateTime(
  utcIso: string,
  locale: Locale,
  zone: string,
): string {
  assertIanaZone(zone);
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(utcIso));
}

// A stored file's size, for a reader deciding whether to open it — never for
// arithmetic. The unit steps with the file rather than being fixed: a 412-byte
// note rendered on a kilobyte scale reads "0 kB", which looks like a file that
// failed to upload. The unit words ride Intl's own vocabulary, so de and vi
// read correctly without a lookup table here.
export function formatBytes(bytes: number, locale: Locale): string {
  const [value, unit] = byteScale(bytes);
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unit,
    unitDisplay: "short",
    maximumFractionDigits: unit === "byte" ? 0 : 1,
  }).format(value);
}

function byteScale(bytes: number): [number, "byte" | "kilobyte" | "megabyte"] {
  if (bytes >= 1_000_000) {
    return [bytes / 1_000_000, "megabyte"];
  }
  if (bytes >= 1000) {
    return [bytes / 1000, "kilobyte"];
  }
  return [bytes, "byte"];
}

/**
 * calendarDaysBetween counts whole days between two instants by the CALENDAR,
 * in UTC — the frontend spelling of the backend's shared/kernel/elapsed.Days.
 *
 * Not by elapsed milliseconds, and the difference is a defect the product has
 * already shipped once. Two surfaces on the deal page counted the same silence
 * and printed 96 and 95 on one card, because one counted calendar days and the
 * other divided a duration by 24 hours; they agreed only around midnight. The
 * server now counts one way, and a screen that counted the other way here
 * would put the disagreement straight back on the page.
 *
 * Held by: format.calendar-days.test.ts, which pins the boundary case the
 * millisecond spelling gets wrong (23:00 to 01:00 is one day, not zero).
 */
export function calendarDaysBetween(from: Date, to: Date): number {
  const day = 86_400_000;
  const fromDay = Math.floor(from.getTime() / day);
  const toDay = Math.floor(to.getTime() / day);
  return toDay - fromDay;
}

/**
 * relativeDays reads a timestamp the way a person says it: today, yesterday,
 * or "N days".
 *
 * `never` is reserved for a read that HAPPENED and found nothing — the caller
 * decides that by passing null only when the section was readable. A withheld
 * section is not "never"; it is unknown, and its card says so.
 *
 * It lives here because three screens wanted it and each wrote its own: two
 * private copies in personstrip.tsx and personrail.tsx, byte-equivalent and
 * both counting milliseconds. format.ts's own INTL_LOCALE comment had been
 * pointing at this gap since it was written — "exported because a relative-time
 * formatter needs the same mapping".
 */
export function relativeDays(
  at: string | null | undefined,
  t: (key: MessageKey, vars?: Record<string, string | number>) => string,
  now: Date = new Date(),
): string {
  if (!at) {
    return t("person.strip.never");
  }
  const days = calendarDaysBetween(new Date(at), now);
  if (days <= 0) {
    return t("person.strip.today");
  }
  if (days === 1) {
    return t("person.strip.yesterday");
  }
  return t("person.strip.days", { count: days });
}

// Idle/SLA spans display as ABSOLUTE durations (no naive calendar diff —
// architecture/10 §2): the input is a millisecond span already computed
// upstream from two UTC instants.
export function formatDuration(ms: number, locale: Locale): string {
  const days = Math.floor(ms / 86_400_000);
  const hours = Math.floor((ms % 86_400_000) / 3_600_000);
  const unit = new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unit: "day",
    unitDisplay: "narrow",
  });
  if (days >= 1) {
    return unit.format(days);
  }
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unit: "hour",
    unitDisplay: "narrow",
  }).format(hours);
}

// FX lineage (ADR-0004): a converted figure ships with its contributing rows
// from the query-plan IR. The UI consumes base_value_minor VERBATIM — it
// never multiplies native × rate and never fetches a rate.
export type FxLineageRow = {
  label: string;
  nativeAmountMinor: number;
  nativeCurrency: string;
  rate: number;
  rateDate: string;
};

export type ExplainedMoney = {
  baseValueMinor: number;
  baseCurrency: string;
  rows: FxLineageRow[];
};
