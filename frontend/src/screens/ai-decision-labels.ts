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

type CodeWords = Readonly<{
  // Standing alone: a badge, a name over its code.
  name: MessageKey;
  // Inside a sentence, after a count: "3 timed out".
  phrase: MessageKey;
  // Not a failure: the call was served, or put off to run later.
  warns?: true;
}>;

// Every code `classifyError` files on a call (backend ai/callstore.go), and the
// reasons a decision walk gives up on. ai-decision-labels.test.ts reads the Go.
const CALL_CODES: Readonly<Record<string, CodeWords>> = {
  provider_error: {
    name: "aicalls.sentinel.provider_error",
    phrase: "aiOutcome.gaveUp.failed",
  },
  provider_quota: {
    name: "aicalls.sentinel.provider_quota",
    phrase: "aiOutcome.gaveUp.quota",
  },
  provider_throttled: {
    name: "aicalls.sentinel.provider_throttled",
    phrase: "aiOutcome.gaveUp.throttled",
  },
  provider_refused: {
    name: "aicalls.sentinel.provider_refused",
    phrase: "aiOutcome.gaveUp.refused",
  },
  timeout: {
    name: "aicalls.sentinel.timeout",
    phrase: "aiOutcome.gaveUp.timeout",
  },
  output_withheld: {
    name: "aicalls.sentinel.output_withheld",
    phrase: "aiOutcome.gaveUp.withheld",
  },
  output_rejected: {
    name: "aicalls.sentinel.output_rejected",
    phrase: "aiOutcome.gaveUp.invalid",
  },
  request_rejected: {
    name: "aicalls.sentinel.request_rejected",
    phrase: "aiOutcome.gaveUp.rejected",
  },
  request_failed: {
    name: "aicalls.sentinel.request_failed",
    phrase: "aiOutcome.gaveUp.notSent",
  },
  budget_deferred: {
    name: "aicalls.sentinel.budget_deferred",
    phrase: "aiOutcome.gaveUp.deferred",
    warns: true,
  },
  budget_unavailable: {
    name: "aicalls.sentinel.budget_unavailable",
    phrase: "aiOutcome.gaveUp.budgetUnavailable",
  },
  metering_failed: {
    name: "aicalls.sentinel.metering_failed",
    phrase: "aiOutcome.gaveUp.meteringFailed",
    warns: true,
  },
  decision_error: {
    name: "aicalls.reason.decision_error",
    phrase: "aiOutcome.gaveUp.failed",
  },
  decision_below_floor: {
    name: "aicalls.reason.decision_below_floor",
    phrase: "aiOutcome.gaveUp.unsure",
  },
  decision_off_enum: {
    name: "aicalls.reason.decision_off_enum",
    phrase: "aiOutcome.gaveUp.offEnum",
  },
  schema_invalid: {
    name: "aicalls.sentinel.schema_invalid",
    phrase: "aiOutcome.gaveUp.invalid",
  },
};

export const KNOWN_CALL_CODES: readonly string[] = Object.keys(CALL_CODES);

/** A code standing alone. The server adds codes, so an unknown one is some failure. */
export function callCodeName(code: string, t: T): string {
  const words = CALL_CODES[code];
  return words ? t(words.name) : t("aicalls.outcome.failed");
}

/** Whether the code is one this screen has words for; an unknown one shows its code. */
export function isKnownCallCode(code: string): boolean {
  return Object.hasOwn(CALL_CODES, code);
}

export function callCodeTone(code: string): "warning" | "danger" {
  return CALL_CODES[code]?.warns ? "warning" : "danger";
}

/** A code inside a sentence; an unknown one is quoted as sent. */
export function gaveUpLabel(code: string, t: T): string {
  const words = CALL_CODES[code];
  return words ? t(words.phrase) : code;
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
