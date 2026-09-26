// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { INTL_LOCALE } from "../format/format";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// The factors this build has a word for.
//
// A Map rather than an object literal, because the lookup key arrives off the
// wire: an object inherits from `Object.prototype`, so a run naming the factor
// `toString` resolves to a FUNCTION, passes an `=== undefined` guard and is
// handed to the translator as a message key. A Map has no prototype to reach
// through, and its miss is the `undefined` the unnamed arm is written for.
const FACTOR_NAMES: ReadonlyMap<string, MessageKey> = new Map([
  ["warmth", "brief.factor.warmth"],
]);

/**
 * omittedFactorsText names the factors a run could not weigh, or null when it
 * weighed them all.
 *
 * It says only WHAT was left out, never why. `warmth` is missing because a
 * grant withheld it, but the array carries no reason and any other token is by
 * construction one this build knows nothing about — including nothing about
 * why the server dropped it. Naming a cause would guess, and the caveat
 * deliberately carries no retry, so guessing "your role" would state a false
 * cause and withhold the remedy for the real one.
 *
 * Names are deduplicated and joined by the reader's own conjunction, the way
 * `categoryNamesTogether` joins a pair: the wire field carries no
 * `uniqueItems`, and a line naming one factor twice reads as a fault.
 */
export function omittedFactorsText(
  factors: readonly string[],
  t: Translator,
  locale: Locale,
): string | null {
  if (factors.length === 0) {
    return null;
  }
  const named = new Set<string>();
  let unnamed = false;
  for (const factor of factors) {
    const key = FACTOR_NAMES.get(factor);
    if (key === undefined) {
      unnamed = true;
    } else {
      named.add(t(key));
    }
  }
  if (unnamed) {
    named.add(t("brief.factor.unknown"));
  }
  const together = new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "long",
    type: "conjunction",
  }).format(named);
  return t("brief.order.withheld", { factors: together });
}
