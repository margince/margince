// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { useT } from "../i18n";

type T = ReturnType<typeof useT>;

// The tier the decision lane stamps on its calls. Not a tier of the ladder: the
// task contract never declares it, so it is spelled here rather than looked up.
const DECIDE_TIER = "decide";

// A tier as a reader meets it in a table. Every ladder tier is shown as the
// routing document spells it, because that is the word an operator greps for;
// `decide` alone is named, since it is no tier anybody bound and the bare word
// reads as a verb in a column of nouns.
export function tierLabel(tier: string, t: T): string {
  return tier === DECIDE_TIER ? t("aiTier.decide") : tier;
}

// Why an attempt ran, where the answer is that the decision model before it did
// not stand. An explicit switch rather than a key built from the wire string:
// the catalog is a closed union, and the reason field is a free string the
// server widens without notice. Anything else is shown as sent — an ordinary
// fallback reason is already an operator's word, and an unknown one is "some
// reason", never a refusal.
export function attemptReasonLabel(reason: string, t: T): string {
  switch (reason) {
    case "decision_below_floor":
      return t("aicalls.reason.decision_below_floor");
    case "decision_error":
      return t("aicalls.reason.decision_error");
    case "decision_off_enum":
      return t("aicalls.reason.decision_off_enum");
    case "decision_state_too_large":
      return t("aicalls.reason.decision_state_too_large");
    case "decision_uncertified":
      return t("aicalls.reason.decision_uncertified");
    case "decision_local_only":
      return t("aicalls.reason.decision_local_only");
    default:
      return reason || "—";
  }
}

type SkipReason = NonNullable<
  components["schemas"]["AiFeatureRoute"]["decision_skip_reason"]
>;

// Why a feature that declares a decision form is not answered by the decision
// model. A closed enum on the wire, so the switch is exhaustive and a new value
// fails the typecheck here rather than rendering blank.
export function decisionSkipLabel(reason: SkipReason, t: T): string {
  switch (reason) {
    case "unbound":
      return t("aiAdmin.decisionSkip.unbound");
    case "uncertified":
      return t("aiAdmin.decisionSkip.uncertified");
    case "local_only":
      return t("aiAdmin.decisionSkip.local_only");
  }
}
