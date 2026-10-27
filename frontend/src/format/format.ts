import type { Locale, Translator } from "../i18n";
import { calendarDaysBetween } from "./format.dates";
// The presentation edge (architecture/10 §1–3): everything here formats
// ALREADY-stored values — minor units, UTC instants, IR-provided base
// aggregates. No FX math, no live rate calls, no calendar arithmetic, and
// locale never flows back into storage. format.test.ts pins each rule.
import { INTL_LOCALE } from "./intllocale";
import { minorUnitDigits, toMajorUnits } from "./minorunits";
import { formatMoneyOrWord } from "./moneyword";

export {
  assertIanaZone,
  calendarDaysBetween,
  displayDay,
  formatDate,
  formatDateAbbrev,
  formatDateTime,
  formatDayLong,
  formatDayMonth,
  formatDayMonthNearYear,
  formatDayRange,
  formatTimeOfDay,
  fullDayName,
  hourInZone,
  isRenderableZone,
  monthAndYear,
  monthName,
  weekdayInitial,
} from "./format.dates";
export { INTL_LOCALE } from "./intllocale";

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

/**
 * A money figure as a record page's rows write it: whole units when the amount
 * has no cents (€48,000), the exact figure otherwise.
 */
export function formatMoneyWhole(
  amountMinor: number,
  currency: string,
  locale: Locale,
): string {
  const major = toMajorUnits(amountMinor, currency);
  if (!Number.isInteger(major)) {
    return formatMoney(amountMinor, currency, locale);
  }
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(major);
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
  // The dash is one WORD for the absence among several, so both spellings run
  // on one guard rather than agreeing twice about what "no money here" means.
  return formatMoneyOrWord(
    amountMinor,
    currency,
    locale,
    MONEY_ABSENT,
    formatMoney,
  );
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
 * Under 10,000 it stays exact, cents included: "€8,332" is as short as "€8k"
 * and says more, and a one-cent deal rounded to "€0" reads as no value.
 */
export function formatMoneyCompact(
  amountMinor: number,
  currency: string,
  locale: Locale,
): string {
  // Our scale, Intl's formatting: see formatMoney for why.
  const major = toMajorUnits(amountMinor, currency);
  const abbreviated = Math.abs(major) >= 10_000;
  if (!abbreviated && !Number.isInteger(major)) {
    return formatMoney(amountMinor, currency, locale);
  }
  const vietnameseCompact = locale === "vi" && abbreviated;
  const scale =
    Math.abs(major) >= 1_000_000_000
      ? 1_000_000_000
      : Math.abs(major) >= 1_000_000
        ? 1_000_000
        : 1_000;
  const unit = scale === 1_000 ? "K" : scale === 1_000_000 ? " triệu" : " tỷ";
  const formatter = new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "currency",
    currency,
    notation: abbreviated && !vietnameseCompact ? "compact" : "standard",
    // Spelled out: the currency style starts the minimum at the currency's own
    // digits, so a maximum of 1 alone always prints one digit ("€420.0k").
    minimumFractionDigits: 0,
    maximumFractionDigits: abbreviated ? 1 : 0,
    // Half-up turns CHF 999,999.99 into "CHF 1M", a figure the record does
    // not reach; truncating drops digits but never adds any.
    roundingMode: "trunc",
  });
  if (!vietnameseCompact) return formatter.format(major);
  const parts = formatter.formatToParts(major / scale);
  const lastDigit = parts.reduce(
    (last, part, index) =>
      part.type === "integer" || part.type === "fraction" ? index : last,
    -1,
  );
  return parts
    .map((part, index) => part.value + (index === lastDigit ? unit : ""))
    .join("");
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

/**
 * A share of a whole where the FRACTION matters: "0.4 %", "12.0 %".
 *
 * `formatPercent` rounds to whole numbers, which is right for progress. It is
 * wrong for a rate held to a threshold below one percent — a transition sitting
 * at 0.4% undone would print "0%" and read as flawless, which is the rounding
 * that flatters exactly where flattery is dangerous.
 *
 * Zero is still "0 %": nothing went wrong is a whole fact, and "0.0 %" claims a
 * measurement where there is simply nothing to measure.
 */
export function formatFinePercent(fraction: number, locale: Locale): string {
  if (fraction === 0) {
    return formatPercent(0, locale);
  }
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "percent",
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(fraction);
}

export function formatNumber(value: number, locale: Locale): string {
  return new Intl.NumberFormat(INTL_LOCALE[locale]).format(value);
}

// A measured span in hours, to one decimal and in the reader's unit word: a
// median first response of 3.5 hours reads "3.5 hr", "3,5 Std." or "3,5 giờ".
export function formatHours(hours: number, locale: Locale): string {
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unit: "hour",
    unitDisplay: "short",
    maximumFractionDigits: 1,
  }).format(hours);
}

/**
 * A share of a whole as a whole-number percentage: "42%", or "42 %" for a
 * German reader, whose typography puts a space before the sign.
 *
 * Whole numbers only, because what this labels is progress — a ring, a chip —
 * where "41.7%" claims a precision a count of scanned messages does not have.
 * A rate that carries a real fraction is `formatFinePercent`'s job.
 */
export function formatPercent(fraction: number, locale: Locale): string {
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "percent",
    maximumFractionDigits: 0,
  }).format(fraction);
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
 * A CHANGE in money: "+€3,000", "-€1,200", "±€0".
 *
 * The money counterpart to `formatSignedNumber`, and it keeps both of that
 * function's rules — the sign always shows, because a bare figure beside last
 * week's leaves the direction to be guessed; and an exactly level result prints
 * "±" rather than "+", because a week that matched the one before is a real
 * answer and dressing it as growth is a small lie repeated weekly.
 *
 * It goes through the same minor-unit conversion `formatMoney` uses rather than
 * dividing by a hundred: the fraction digits differ by currency, and a yen
 * delta rendered with two decimals is off by a factor of a hundred.
 */
export function formatSignedMoney(
  amountMinor: number,
  currency: string,
  locale: Locale,
): string {
  const digits = minorUnitDigits(currency);
  const formatted = new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "currency",
    currency,
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
    signDisplay: "always",
  }).format(toMajorUnits(amountMinor, currency));
  // Intl has no "±", so the zero case is spelled here — over the FORMATTED
  // string, so the currency symbol keeps whatever position the locale gives it.
  return amountMinor === 0 ? formatted.replace("+", "±") : formatted;
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

// A stored file's size, for a reader deciding whether to open it — never for
// arithmetic. The unit steps with the file rather than being fixed. A 412-byte
// note on a kilobyte scale reads "0 kB", which looks like a failed upload.
// Intl's own unit words serve de and vi. A byte takes the long form, because
// the short English one never inflects ("98 byte").
export function formatBytes(bytes: number, locale: Locale): string {
  const [value, unit] = byteScale(bytes);
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unit,
    unitDisplay: unit === "byte" ? "long" : "short",
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
 * relativeDays reads a timestamp the way a contact says it: today, yesterday,
 * or "N days".
 *
 * `never` is reserved for a read that HAPPENED and found nothing — the caller
 * decides that by passing null only when the section was readable. A withheld
 * section is not "never"; it is unknown, and its card says so.
 *
 * It lives here because three screens wanted it and each wrote its own: two
 * private copies in contactstrip.tsx and contactrail.tsx, byte-equivalent and
 * both counting milliseconds. format.ts's own INTL_LOCALE comment had been
 * pointing at this gap since it was written — "exported because a relative-time
 * formatter needs the same mapping".
 */
export function relativeDays(
  at: string | null | undefined,
  t: Translator,
  locale: Locale,
  zone: string,
  now: Date = new Date(),
): string {
  if (!at) {
    return t("contact.strip.never");
  }
  return daysAgoText(calendarDaysBetween(new Date(at), now, zone), t, locale);
}

/** A count of dates since a moment, as relativeDays says it. */
export function daysAgoText(
  days: number,
  t: Translator,
  locale: Locale,
): string {
  if (days <= 0) {
    return t("contact.strip.today");
  }
  if (days === 1) {
    return t("contact.strip.yesterday");
  }
  // The count is a MAGNITUDE, so it is grouped in the reader's own notation.
  // Handed to `t` as a raw number it reached the catalog sentence through
  // string coercion, which groups for nobody: a contact last written to 1200
  // days ago read "1200 days" in a German sentence that spells every other
  // figure on the page "1.200".
  return t("contact.strip.days", { count: formatNumber(days, locale) });
}

// Response times and timers measure elapsed duration; record ages use formatAge.
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

/** Age on the record calendar, retaining hours for events earlier today. */
export function formatAge(
  at: string,
  now: number,
  locale: Locale,
  zone: string,
): string {
  const days = Math.max(
    0,
    calendarDaysBetween(new Date(at), new Date(now), zone),
  );
  const hours = Math.max(0, Math.floor((now - Date.parse(at)) / 3_600_000));
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unit: days > 0 ? "day" : "hour",
    unitDisplay: "narrow",
  }).format(days > 0 ? days : hours);
}
