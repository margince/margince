// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { INTL_LOCALE } from "./intllocale";

// The one place this tree builds an Intl formatter. Building one costs far
// more than one format() call, and a list page asks the same question per row.
// `census/rules/one-formatter-cache.census.ts` fails a constructor elsewhere.

// A date formatter with a zone: the zone is part of the key, so a cached one
// never answers for another zone.
export type ZonedDateTimeOptions = Intl.DateTimeFormatOptions & {
  timeZone: string;
};

// An option left undefined is left out, as Intl reads it.
function keyOf(kind: string, tag: string, options: object | undefined) {
  const set = Object.entries(options ?? {})
    .filter(([, value]) => value !== undefined)
    .sort(([a], [b]) => (a < b ? -1 : 1));
  return `${kind}|${tag}|${JSON.stringify(set)}`;
}

// One store per kind, so a read needs no cast. Unbounded by design: the keys
// are three locales times the option shapes written here, currencies and zones.
function remember<T>(store: Map<string, T>, key: string, build: () => T): T {
  const found = store.get(key);
  if (found !== undefined) return found;
  const made = build();
  store.set(key, made);
  return made;
}

const numbers = new Map<string, Intl.NumberFormat>();
const dates = new Map<string, Intl.DateTimeFormat>();
const lists = new Map<string, Intl.ListFormat>();
const relatives = new Map<string, Intl.RelativeTimeFormat>();
const names = new Map<string, Intl.DisplayNames>();
const plurals = new Map<string, Intl.PluralRules>();
const collators = new Map<string, Intl.Collator>();
const segmenters = new Map<string, Intl.Segmenter>();

export function numberFormat(
  locale: Locale,
  options?: Intl.NumberFormatOptions,
): Intl.NumberFormat {
  return remember(
    numbers,
    keyOf("number", locale, options),
    () => new Intl.NumberFormat(INTL_LOCALE[locale], options),
  );
}

// A formatter built without a zone keeps the browser zone it was built in,
// so a cached one would miss a later change of zone. Refused here.
function requireZone(options: ZonedDateTimeOptions): void {
  // RangeError, as Intl throws for a zone it cannot use.
  if (typeof options.timeZone !== "string" || options.timeZone === "") {
    throw new RangeError(
      "a cached date formatter needs a timeZone; use zonelessDateTimeFormat",
    );
  }
}

export function dateTimeFormat(
  locale: Locale,
  options: ZonedDateTimeOptions,
): Intl.DateTimeFormat {
  requireZone(options);
  return remember(
    dates,
    keyOf("date", locale, options),
    () => new Intl.DateTimeFormat(INTL_LOCALE[locale], options),
  );
}

/**
 * A date formatter in a fixed tag, for output read back as numbers and never
 * shown: en-CA writes YYYY-MM-DD, en-US with h23 a wall clock to parse.
 */
export function pinnedDateTimeFormat(
  tag: "en-CA" | "en-US",
  options: ZonedDateTimeOptions,
): Intl.DateTimeFormat {
  requireZone(options);
  return remember(
    dates,
    keyOf("pinned-date", tag, options),
    () => new Intl.DateTimeFormat(tag, options),
  );
}

/** Built on every call, so the browser's zone is read again each time. */
export function zonelessDateTimeFormat(
  locale: Locale,
  options: Intl.DateTimeFormatOptions & { timeZone?: never },
): Intl.DateTimeFormat {
  return new Intl.DateTimeFormat(INTL_LOCALE[locale], options);
}

export function listFormat(
  locale: Locale,
  options?: Intl.ListFormatOptions,
): Intl.ListFormat {
  return remember(
    lists,
    keyOf("list", locale, options),
    () => new Intl.ListFormat(INTL_LOCALE[locale], options),
  );
}

export function relativeTimeFormat(
  locale: Locale,
  options?: Intl.RelativeTimeFormatOptions,
): Intl.RelativeTimeFormat {
  return remember(
    relatives,
    keyOf("relative", locale, options),
    () => new Intl.RelativeTimeFormat(INTL_LOCALE[locale], options),
  );
}

export function displayNames(
  locale: Locale,
  options: Intl.DisplayNamesOptions,
): Intl.DisplayNames {
  return remember(
    names,
    keyOf("names", locale, options),
    () => new Intl.DisplayNames(INTL_LOCALE[locale], options),
  );
}

export function pluralRules(locale: Locale): Intl.PluralRules {
  return remember(
    plurals,
    keyOf("plural", locale, undefined),
    () => new Intl.PluralRules(INTL_LOCALE[locale]),
  );
}

export function collator(locale: Locale): Intl.Collator {
  return remember(
    collators,
    keyOf("collator", locale, undefined),
    () => new Intl.Collator(INTL_LOCALE[locale]),
  );
}

export function segmenter(
  locale: Locale,
  options?: Intl.SegmenterOptions,
): Intl.Segmenter {
  return remember(
    segmenters,
    keyOf("segmenter", locale, options),
    () => new Intl.Segmenter(INTL_LOCALE[locale], options),
  );
}

/** Grapheme clusters are Unicode's and the same in every locale. */
export function graphemeSegmenter(): Intl.Segmenter {
  return remember(
    segmenters,
    keyOf("segmenter", "", { granularity: "grapheme" }),
    () => new Intl.Segmenter(undefined, { granularity: "grapheme" }),
  );
}
