import type { Locale } from "../i18n";
import { pluralRules } from "./formatters";

// Which plural form a count takes, in the reader's language.
//
// This lives beside the other locale-sensitive readings for the reason
// `census/rules/one-locale.census.ts` states about all of them. The question is
// never whether a rendering is locale-sensitive, only whose locale it uses.
// The answer here is the reader's own, through `INTL_LOCALE`. A count
// comparison written at the call site answers it differently. It answers
// "English and German", which is
// two of the three languages this product ships.
//
// The three shipped locales need at most two forms, so nothing is visibly wrong
// today and nothing here changes what they render. What changes is where the
// rule lives: Polish has four categories, Arabic six, and a `count === 1`
// written at fifteen call sites is fifteen separate wrongs the day one of them
// ships, with no single place to fix them.

/**
 * The CLDR plural category a count falls in for this locale.
 *
 * `Intl.PluralRules.select` refuses nothing, but a non-finite count would
 * produce a category off a number no reader has — so a caller that has not got
 * a number yet asks with the count it means to render, not with `NaN`.
 */
export function pluralCategory(
  locale: Locale,
  count: number,
): Intl.LDMLPluralRule {
  if (!Number.isFinite(count)) {
    throw new Error(`plural category asked for a non-finite count: ${count}`);
  }
  return pluralRules(locale).select(count);
}
