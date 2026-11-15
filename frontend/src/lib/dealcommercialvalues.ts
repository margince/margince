import type { components } from "../api/schema";

type UpdateDealRequest = components["schemas"]["UpdateDealRequest"];

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
