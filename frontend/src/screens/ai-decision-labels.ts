// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

type T = ReturnType<typeof useT>;

// The rung the decision lane stamps on its calls (decidetrace.go). Not a tier
// of the ladder: the task contract never declares it, so it is spelled here
// once and every screen that meets it in a health or usage row imports it.
export const DECIDE_RUNG = "decide";

// Cheapest first. Typed by the task contract's enum, so a tier it grows
// without a name here fails the typecheck.
export type Tier = components["schemas"]["AssistantConfiguredModel"]["tier"];

const TIER_NAMES = {
  local_small: "aiTier.local_small",
  cheap_cloud: "aiTier.cheap_cloud",
  premium: "aiTier.premium",
  frontier: "aiTier.frontier",
  local_large: "aiTier.local_large",
} as const satisfies Record<Tier, MessageKey>;

// The lanes bound beside the ladder, under their routing keys and the rungs
// their calls are stamped with (embedlane.go, decidetrace.go).
const LANE_NAMES: Readonly<Record<string, MessageKey>> = {
  embeddings: "aiTier.embeddings",
  embed: "aiTier.embeddings",
  decisions: "aiTier.decide",
  [DECIDE_RUNG]: "aiTier.decide",
};

function isTier(key: string): key is Tier {
  return Object.hasOwn(TIER_NAMES, key);
}

export const TIER_ORDER: readonly Tier[] =
  Object.keys(TIER_NAMES).filter(isTier);

export function tierLabel(tier: string, t: T): string {
  if (isTier(tier)) return t(TIER_NAMES[tier]);
  const lane = LANE_NAMES[tier];
  return lane ? t(lane) : tier;
}

export function tierRank(tier: string): number {
  return isTier(tier) ? TIER_ORDER.indexOf(tier) : TIER_ORDER.length;
}

const GAVE_UP: Readonly<Record<string, MessageKey>> = {
  timeout: "aiOutcome.gaveUp.timeout",
  provider_error: "aiOutcome.gaveUp.failed",
  provider_throttled: "aiOutcome.gaveUp.throttled",
  provider_quota: "aiOutcome.gaveUp.quota",
  provider_refused: "aiOutcome.gaveUp.refused",
  decision_error: "aiOutcome.gaveUp.failed",
  decision_below_floor: "aiOutcome.gaveUp.unsure",
  decision_off_enum: "aiOutcome.gaveUp.offEnum",
  schema_invalid: "aiOutcome.gaveUp.invalid",
  output_rejected: "aiOutcome.gaveUp.invalid",
};

export function gaveUpLabel(reason: string, t: T): string {
  const key = GAVE_UP[reason];
  return key ? t(key) : reason;
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
    case "timeout":
      return t("aicalls.reason.timeout");
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
