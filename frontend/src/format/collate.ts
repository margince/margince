// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { INTL_LOCALE } from "./format";

// Two ways to order strings, and the whole point of this file is that a caller
// has to SAY which one it means.
//
// `String.prototype.localeCompare` reads as the careful choice and is the wrong
// default twice over. Called bare it sorts by the BROWSER's guessed locale,
// which is neither the reader's setting nor stable between two colleagues
// looking at the same list. Called with a locale it is right for a name and
// wrong for a key, because a key sorted per-locale keys differently per reader.
//
// So neither answer generalises, which is why there is no single helper here.
// A site picks `forReader` or `stable`, and the name it calls is the record of
// which kind of string it holds.

/**
 * Order two strings the way THIS READER expects to see them.
 *
 * For anything a person reads as a list of names — people, companies, teams,
 * labels. A German reader expects `ä` beside `a`; a Vietnamese reader expects
 * the tone-marked vowels in their own order. Both differ from code-unit order,
 * where `Ä` lands after `Z`.
 *
 * The collator is built per locale and cached, because constructing one costs
 * more than the comparison and a sort calls this O(n log n) times.
 */
export function forReader(a: string, b: string, locale: Locale): number {
  return collator(locale).compare(a, b);
}

const collators = new Map<Locale, Intl.Collator>();

function collator(locale: Locale): Intl.Collator {
  const held = collators.get(locale);
  if (held) {
    return held;
  }
  const made = new Intl.Collator(INTL_LOCALE[locale]);
  collators.set(locale, made);
  return made;
}

/**
 * Order two strings IDENTICALLY for everyone, forever.
 *
 * For a machine key, an id, an ISO timestamp, a currency code, an enum value —
 * and for the tiebreaker that keeps a sort from re-ordering itself between two
 * reads of the same data. Code-unit order is not human alphabetical order, and
 * that is not a defect here: nobody reads this as an alphabet, and being the
 * same everywhere is the only property that matters.
 *
 * Spelled out rather than delegating to `localeCompare(a, b, "en")`, so the
 * gate in one-locale.test.ts can refuse every bare `localeCompare` in the tree
 * without this file having to be its exception.
 */
export function stable(a: string, b: string): number {
  if (a < b) {
    return -1;
  }
  return a > b ? 1 : 0;
}

/**
 * Fold a string for MATCHING, identically on every machine.
 *
 * `toLocaleLowerCase` folds per locale — Turkish maps `I` to a dotless `ı` —
 * so a needle folded under one locale stops matching a haystack folded under
 * another. A search is a comparison between two strings this product holds, not
 * a sentence anybody reads, so it wants the invariant fold on both sides.
 */
export function foldForMatch(value: string): string {
  return value.toLowerCase();
}
