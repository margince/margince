import type { Locale, Translator } from "../i18n";
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

/**
 * An FX rate, at whatever precision the record actually carries.
 *
 * Separate from `formatNumber` because that one's Intl default caps at three
 * fraction digits, which SILENTLY drops the rest: a rate stored as 0.9293458
 * renders "0,929", and the reader is then looking at a number the lineage does
 * not hold — in the one place the product exists to show its arithmetic.
 * Separate from `formatDecimal` because that pins the digits open, and a rate
 * of exactly 2 should read "2" rather than "2,0000000000".
 *
 * Ten is the scale the server stores a rate at, so it is the ceiling here.
 */
/**
 * One AI model price, as the sheet spells it: USD per million tokens.
 *
 * Always USD, and not a caller's choice — `ai_model_rate` stores µUSD integers
 * that carry no currency, and the contract calls the four buckets USD outright.
 * A currency argument here would be a knob that can only be set wrong.
 *
 * The FRACTION DIGITS are the whole reason this is not `formatMoney`. That one
 * rounds to the currency's minor unit, which is two digits for USD — and the
 * cheap end of this sheet lives below a cent, where two digits render a real
 * price as `$0.00`. Free is a claim this product is careful never to make by
 * accident (an unpriced call reports UNPRICED for the same reason), so the
 * digits stretch until the first significant one survives, and stop at six
 * because a rate finer than that is not a number anybody reads.
 */
export function formatUsdPerMTok(price: string, locale: Locale): string {
  const value = Number(price);
  const digits =
    value === 0
      ? 2
      : Math.min(6, Math.max(2, Math.ceil(-Math.log10(Math.abs(value)))));
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(value);
}

export function formatRate(value: number, locale: Locale): string {
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    maximumFractionDigits: 10,
  }).format(value);
}

/**
 * The month's own name in the reader's language, from Intl rather than a table
 * of our own — a list of twelve month names per locale is a translation file
 * that goes stale, and the platform already ships them.
 *
 * The day is fixed at the 1st and the year is arbitrary: only the month is
 * rendered, and a month name does not depend on either.
 *
 * `timeZone: "UTC"` is load-bearing, not tidiness. The instant is MINTED in UTC
 * (`Date.UTC`), so formatting it on the reader's own clock reads it back in a
 * different zone than it was written in — and midnight on the 1st is the worst
 * possible instant for that, because any zone behind UTC lands on the last day
 * of the PREVIOUS month. In America/New_York this returned "December" for
 * month 1, so the fiscal-year picker offered a label one month off the value it
 * saved.
 *
 * This is not a moment anybody is reading in their own zone: the argument is a
 * month NUMBER, not a point in time, and the date is scaffolding for Intl's
 * month table. Reading it back in the zone it was written in is what makes the
 * scaffolding cancel out.
 */
export function monthName(month: number, locale: Locale): string {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    month: "long",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(2026, month - 1, 1)));
}

/**
 * The month a calendar grid is showing — "August 2026".
 *
 * A month a reader is LOOKING AT rather than a moment: the grid is built from
 * local dates, so this reads the same local date back rather than pinning a
 * zone the caller never chose.
 */
export function monthAndYear(month: Date, locale: Locale): string {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    month: "long",
    year: "numeric",
  }).format(month);
}

/**
 * A weekday as the one or two letters a calendar's legend uses.
 *
 * From Intl rather than a list of our own: "S M T W T F S" is English, and a
 * hand-written row would sit unchanged over a German or Vietnamese month.
 */
export function weekdayInitial(day: Date, locale: Locale): string {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    weekday: "narrow",
  }).format(day);
}

/**
 * A calendar day written out in full — "Tuesday, 25 August 2026".
 *
 * The accessible name of a day in a grid: "25" alone says nothing about which
 * month it belongs to, and the grid's own heading is several stops away by the
 * time a screen reader arrives at it.
 */
export function fullDayName(day: Date, locale: Locale): string {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    dateStyle: "full",
  }).format(day);
}

export function formatNumber(value: number, locale: Locale): string {
  return new Intl.NumberFormat(INTL_LOCALE[locale]).format(value);
}

/**
 * A CHANGE rather than a quantity: "+3", "-2", "±0".
 *
 * The sign is the whole point, so it is always shown — a bare "3" beside last
 * week's figure leaves a reader to work out which direction it moved, and half
 * of them will guess. Intl's `signDisplay: "always"` renders the plus and the
 * locale's own minus sign, which is not the ASCII hyphen in every locale.
 *
 * Zero prints as "±0" rather than "+0": a week that stayed exactly level is a
 * real answer, and dressing it as an increase is a small lie repeated weekly.
 */
export function formatSignedNumber(value: number, locale: Locale): string {
  if (value === 0) {
    return "±0";
  }
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    signDisplay: "always",
  }).format(value);
}

/**
 * A number that NAMES something rather than measuring it — a version, a
 * revision, an invoice number, a record's own number.
 *
 * Bare on purpose, and this is the ruling rather than the omission of one.
 * `formatNumber` would print revision 1234 as "1.204" to a German reader and
 * "1,204" to an English one, which reads as a quantity of revisions where a
 * name was meant. Grouping a name is as wrong as leaving a magnitude ungrouped;
 * the two are opposite defects and no single answer covers both.
 *
 * It is a FUNCTION and not a bare interpolation because the name is where the
 * decision is recorded. `format/jsx-magnitude.test.ts` refuses a raw number in
 * anything a reader is shown, so every site has had to pick between this,
 * `ordinalNumber` and a formatter — which is what makes the gate absolute
 * instead of a list of blessed exceptions. `format/collate.ts` reached the same
 * shape first, for the same reason: `forReader` and `stable` both order, and
 * only the name says which order was meant.
 */
export function identifierNumber(value: number): string {
  return String(value);
}

/**
 * A number that says WHERE in a sequence — an ordinal, a step, a row's index.
 *
 * Bare for the same reason as `identifierNumber` and kept apart from it because
 * the two say different things to the next reader: a version is a name a
 * server chose, a position is one this list computed, and a site that renames
 * one is not making a claim about the other. See that function for why the
 * ruling is spelled as a call at all.
 */
export function ordinalNumber(value: number): string {
  return String(value);
}

/**
 * A figure whose DECIMALS are part of what it says — a score, a rate, a factor.
 *
 * `formatNumber` renders a count, where a trailing `.0` would be noise. This
 * one pins the fraction digits open, so a scoring breakdown reads "12.50 → 13"
 * with both figures at the precision the reader is being shown them at.
 *
 * The alternative every site reached for first was `value.toFixed(2)`, which is
 * not a formatter: it is locale-blind by construction, so a German reader is
 * shown a decimal POINT where their own numbers carry a comma — and reads
 * "12.50" as twelve hundred and fifty, in a sentence whose next figure is 13.
 */
export function formatDecimal(
  value: number,
  locale: Locale,
  fractionDigits: number,
): string {
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(value);
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
  t: Translator,
  locale: Locale,
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
  // The count is a MAGNITUDE, so it is grouped in the reader's own notation.
  // Handed to `t` as a raw number it reached the catalog sentence through
  // string coercion, which groups for nobody: a person last written to 1200
  // days ago read "1200 days" in a German sentence that spells every other
  // figure on the page "1.200".
  return t("person.strip.days", { count: formatNumber(days, locale) });
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

/**
 * The clock hour, 0-23, that an instant reads as in one zone.
 *
 * Here rather than at the call site because this file is where a formatter is
 * constructed: outside it, a locale reaches `Intl` through `INTL_LOCALE` or it
 * does not reach one at all (`one-locale.test.ts` holds that). The hour itself
 * carries no locale — 08 is 08 in every language — so the tag is `en-GB` for
 * `hourCycle: "h23"` to be unambiguous, and the ZONE is what the caller is
 * actually asking about.
 *
 * Used to decide which greeting a reader gets, which is a fact about their
 * morning rather than about any record.
 */
export function hourInZone(instant: Date, zone: string): number {
  const hour = new Intl.DateTimeFormat(INTL_LOCALE.en, {
    hour: "numeric",
    hourCycle: "h23",
    timeZone: zone,
  }).format(instant);
  return Number.parseInt(hour, 10);
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
