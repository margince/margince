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
// The value type admits `undefined` because the compiler cannot: the contract
// pins the stored vocabulary to `warmth`, so a token outside this map reaches a
// reader only from a NEWER server. Typed as a total record it would read as
// exhaustive and the unnamed arm below would look like dead code.
const FACTOR_NAMES: Readonly<Record<string, MessageKey | undefined>> = {
  warmth: "brief.factor.warmth",
};

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
    const key = FACTOR_NAMES[factor];
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
