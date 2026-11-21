import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Deal = components["schemas"]["Deal"];

// The words for the deal columns that hold a closed set of codes. The deal
// forms, its filters and a record's history all read them here.
export const DEAL_MOTION_LABEL: Readonly<
  Record<NonNullable<Deal["commercial_motion"]>, MessageKey>
> = {
  new_business: "deal.motionNewBusiness",
  renewal: "deal.motionRenewal",
  upsell: "deal.motionUpsell",
  cross_sell: "deal.motionCrossSell",
  expansion: "deal.motionExpansion",
  existing_business: "deal.motionExistingBusiness",
};

export const DEAL_PRIORITY_LABEL: Readonly<
  Record<NonNullable<Deal["priority"]>, MessageKey>
> = {
  high: "deal.priorityHigh",
  medium: "deal.priorityMedium",
  low: "deal.priorityLow",
};

export const DEAL_ATTRIBUTION_LABEL: Readonly<
  Record<NonNullable<Deal["partner_attribution"]>, MessageKey>
> = {
  sourced: "deal.attributionSourced",
  influenced: "deal.attributionInfluenced",
};

// A select's options: the empty choice first, then each code in the order the
// map lists them.
export function withUnset(
  labels: Readonly<Record<string, MessageKey>>,
  unset: MessageKey,
): { value: string; label: MessageKey }[] {
  return [
    { value: "", label: unset },
    ...Object.entries(labels).map(([value, label]) => ({ value, label })),
  ];
}
