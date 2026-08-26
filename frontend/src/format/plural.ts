import type { Locale } from "../i18n";
import { INTL_LOCALE } from "./format";

// Which plural form a count takes, in the reader's language.
//
// This lives beside the other locale-sensitive readings for the reason
// `one-locale.test.ts` states about all of them: the question is never WHETHER a
// rendering is locale-sensitive, only whose locale it uses, and the answer here
// is the reader's own through `INTL_LOCALE`. A count comparison written at the
// call site answers it differently — it answers "English and German", which is
// two of the three languages this product ships.
//
// The three shipped locales need at most two forms, so nothing is visibly wrong
// today and nothing here changes what they render. What changes is where the
// rule lives: Polish has four categories, Arabic six, and a `count === 1`
// written at fifteen call sites is fifteen separate wrongs the day one of them
// ships, with no single place to fix them.

// One instance per locale rather than one per call: constructing an
// Intl.PluralRules is the expensive half, and selecting from it is not.
const RULES = new Map<Locale, Intl.PluralRules>();

function rulesFor(locale: Locale): Intl.PluralRules {
  const existing = RULES.get(locale);
  if (existing) {
    return existing;
  }
  const rules = new Intl.PluralRules(INTL_LOCALE[locale]);
  RULES.set(locale, rules);
  return rules;
}

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
  return rulesFor(locale).select(count);
}
