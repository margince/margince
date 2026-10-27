// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { isRealCalendarDay } from "../format/calendarday";
import { fiscalQuarter } from "./fiscalquarter";
import { problemFieldErrorsOf } from "./problem";

// The deals list's range dials, as the address and a saved view hold them.
//
// A saved view keeps each filter as one string. So a range is held as
// "low..high" with either end blank, and the wire's two bounds are read here.
// "this_quarter" stays a word rather than two days: a view saved on it still
// means the current quarter after the quarter turns.

/** The close-day dial: a "from..to" pair of days, or THIS_QUARTER. */
export const CLOSE_DAY_DIAL = "close_day";
/** The value dial: a "min..max" pair in minor units of the base currency. */
export const VALUE_DIAL = "value";
export const THIS_QUARTER = "this_quarter";

const RANGE_SEPARATOR = "..";

export type DayRange = Readonly<{ from?: string; to?: string }>;
export type MinorRange = Readonly<{ min?: number; max?: number }>;

/** Joins two ends into one dial value; "" when both are blank. */
export function encodeRange(low: string, high: string): string {
  return low || high ? `${low}${RANGE_SEPARATOR}${high}` : "";
}

function splitRange(value: string | undefined): [string, string] | undefined {
  const parts = value?.split(RANGE_SEPARATOR);
  if (parts?.length !== 2) return undefined;
  const [low = "", high = ""] = parts;
  return low || high ? [low, high] : undefined;
}

/** A "from..to" day range, or undefined for anything else. */
export function parseDayRange(value: string | undefined): DayRange | undefined {
  const ends = splitRange(value);
  if (!ends) return undefined;
  const [from, to] = ends;
  if ((from && !isRealCalendarDay(from)) || (to && !isRealCalendarDay(to))) {
    return undefined;
  }
  if (from && to && from > to) return undefined;
  return { from: from || undefined, to: to || undefined };
}

const MINOR = /^\d+$/;

/** A "min..max" minor-unit range, or undefined for anything else. */
export function parseMinorRange(
  value: string | undefined,
): MinorRange | undefined {
  const ends = splitRange(value);
  if (!ends) return undefined;
  const [low, high] = ends;
  if ((low && !MINOR.test(low)) || (high && !MINOR.test(high))) {
    return undefined;
  }
  const min = low ? Number(low) : undefined;
  const max = high ? Number(high) : undefined;
  if (min !== undefined && max !== undefined && min > max) return undefined;
  return { min, max };
}

/** The dial's value as a day range, with THIS_QUARTER counted from `today`. */
export function closeDayRange(
  value: string | undefined,
  today: string,
  fiscalStartMonth: number,
): DayRange | undefined {
  return value === THIS_QUARTER
    ? fiscalQuarter(today, fiscalStartMonth)
    : parseDayRange(value);
}

/** Whether a stored dial value is one the list can send. */
export function admitCloseDay(value: string | undefined) {
  return value === THIS_QUARTER || parseDayRange(value) ? value : undefined;
}

export function admitValueRange(value: string | undefined) {
  return parseMinorRange(value) ? value : undefined;
}

export function admitCurrency(value: string | undefined) {
  return value && /^[A-Z]{3}$/.test(value) ? value : undefined;
}

export function admitCountry(value: string | undefined) {
  return value && /^[A-Z]{2}$/.test(value) ? value : undefined;
}

/**
 * The wire's bounds for the range dials. `closeDay` is the close-day dial
 * already read as days, since only the screen knows today and the fiscal year.
 */
export function rangeQueryParams(
  filters: Readonly<Record<string, string>>,
  closeDay: DayRange | undefined,
) {
  const value = parseMinorRange(filters[VALUE_DIAL]);
  return {
    close_from: closeDay?.from,
    close_to: closeDay?.to,
    value_min: value?.min,
    value_max: value?.max,
    currency: admitCurrency(filters.currency),
    country: admitCountry(filters.country),
  };
}

/** What the reader's grants let a deal dial read, as the screen knows them. */
export type DialReach = Readonly<{
  baseCurrency: string | undefined;
  /** Whether a company can be searched for, which the company dial needs. */
  readsCompanies: boolean;
  /** The list filters `/me` says this reader's role is refused. */
  refused: readonly string[];
}>;

// The dials whose wire parameters carry another name: each range sends a pair
// of bounds. Every other dial is sent under its own key.
const DIAL_OF_PARAMETER: Readonly<Record<string, string>> = {
  close_from: CLOSE_DAY_DIAL,
  close_to: CLOSE_DAY_DIAL,
  value_min: VALUE_DIAL,
  value_max: VALUE_DIAL,
};

/** The dial that sends a deal list parameter. */
export function dialOfParameter(parameter: string): string {
  return DIAL_OF_PARAMETER[parameter] ?? parameter;
}

/**
 * The deal dials this reader cannot narrow by. The value is typed in the base
 * currency, a company is picked by searching companies, and the rest the server
 * names.
 */
export function withheldDealDials(reach: DialReach): ReadonlySet<string> {
  const withheld = new Set(reach.refused.map(dialOfParameter));
  if (!reach.baseCurrency) withheld.add(VALUE_DIAL);
  if (!reach.readsCompanies) withheld.add("company_id");
  return withheld;
}

/**
 * The dial a deal list request was refused because the reader's role masks
 * what it reads. Only a snapshot older than the role, or a server older than
 * this client, sends one.
 */
export function maskRefusedDialOf(error: unknown): string | undefined {
  const fault = problemFieldErrorsOf(error).find(
    (each) => each.code === "field_masked",
  );
  return fault && dialOfParameter(fault.field);
}

/**
 * The filters minus every withheld dial, so no dial narrows the list without a
 * chip to clear it. The same object comes back when nothing is dropped.
 */
export function withoutWithheldDials(
  filters: Readonly<Record<string, string>>,
  withheld: ReadonlySet<string>,
): Readonly<Record<string, string>> {
  if (!Object.keys(filters).some((key) => withheld.has(key))) return filters;
  return Object.fromEntries(
    Object.entries(filters).filter(([key]) => !withheld.has(key)),
  );
}

/** Whether any range dial narrows the list. */
export function narrowsByRange(filters: Readonly<Record<string, string>>) {
  return [CLOSE_DAY_DIAL, VALUE_DIAL, "currency", "country"].some((key) =>
    Boolean(filters[key]),
  );
}
