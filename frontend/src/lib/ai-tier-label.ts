// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { labelFor } from "./labelfor";

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

/** A tier or lane by its display name; one this build cannot name shows as sent. */
export function tierLabel(tier: string, t: Translator): string {
  if (isTier(tier)) return t(labelFor(TIER_NAMES, tier));
  const lane = LANE_NAMES[tier];
  return lane ? t(lane) : tier;
}

export function tierRank(tier: string): number {
  return isTier(tier) ? TIER_ORDER.indexOf(tier) : TIER_ORDER.length;
}
