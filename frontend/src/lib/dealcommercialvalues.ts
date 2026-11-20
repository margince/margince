import type { components, operations } from "../api/schema";

type UpdateDealRequest = components["schemas"]["UpdateDealRequest"];
type DealListQuery = NonNullable<
  operations["listDeals"]["parameters"]["query"]
>;

// The deal list's word for "no value recorded", which no enum member can say.
const UNSET = "unset";

/** A motion dial as the deal list admits it, or undefined for none. */
export function motionFilter(
  v: string | undefined,
): DealListQuery["commercial_motion"] {
  return v === UNSET ? UNSET : (commercialMotion(v ?? "") ?? undefined);
}

/** A priority dial as the deal list admits it, or undefined for none. */
export function priorityFilter(
  v: string | undefined,
): DealListQuery["priority"] {
  return v === UNSET ? UNSET : (dealPriority(v ?? "") ?? undefined);
}

export function commercialMotion(
  v: string,
): UpdateDealRequest["commercial_motion"] {
  switch (v) {
    case "new_business":
      return "new_business";
    case "renewal":
      return "renewal";
    case "upsell":
      return "upsell";
    case "cross_sell":
      return "cross_sell";
    case "expansion":
      return "expansion";
    case "existing_business":
      return "existing_business";
    default:
      return null;
  }
}

export function dealPriority(v: string): UpdateDealRequest["priority"] {
  switch (v) {
    case "low":
      return "low";
    case "medium":
      return "medium";
    case "high":
      return "high";
    default:
      return null;
  }
}
