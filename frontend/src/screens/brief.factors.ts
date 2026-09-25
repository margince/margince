// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// What the brief is allowed to CLAIM about its own order, separated from how
// the caveat draws — the same split brief.readings.honesty.ts makes, and for
// the same reason: this is a question about the run's data that a test can ask
// directly, without rendering the band to find out.

// The factors this build has a word for.
//
// A Map rather than an object literal, because the lookup key arrives off the
// wire: an object inherits from `Object.prototype`, so a run naming the factor
// `toString` would resolve to a FUNCTION, pass an `=== undefined` guard and be
// handed to the translator as though it were a message key. A Map has no
// prototype to reach through, and its miss is the `undefined` the unnamed arm
// below is written for.
const FACTOR_NAMES: ReadonlyMap<string, MessageKey> = new Map([
  ["warmth", "brief.factor.warmth"],
]);

/**
 * omittedFactorsText is the sentence a run owes when it could not weigh a
 * ranking factor, or null when it weighed them all.
 *
 * Every unrecognised token collapses into ONE generic phrase rather than one
 * each: a reader learns nothing from hearing twice that this build cannot name
 * something, and a list that repeats itself reads as a fault in the page.
 */
export function omittedFactorsText(
  factors: readonly string[],
  t: Translator,
): string | null {
  if (factors.length === 0) {
    return null;
  }
  const named: string[] = [];
  let unnamed = false;
  for (const factor of factors) {
    const key = FACTOR_NAMES.get(factor);
    if (key === undefined) {
      unnamed = true;
    } else {
      named.push(t(key));
    }
  }
  if (unnamed) {
    named.push(t("brief.factor.unknown"));
  }
  return t("brief.order.withheld", { factors: named.join(", ") });
}
