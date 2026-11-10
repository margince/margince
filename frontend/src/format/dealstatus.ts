import type { operations } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { labelFor } from "../lib/labelfor";

type DealStatus = NonNullable<
  NonNullable<operations["listDeals"]["parameters"]["query"]>["status"]
>;

/** A deal's status in words, once for every screen that names one. */
export const DEAL_STATUS_LABEL: Readonly<Record<DealStatus, MessageKey>> = {
  open: "deals.statusOpen",
  won: "deals.statusWon",
  lost: "deals.statusLost",
};

export const DEAL_STATUS_VALUES = Object.keys(
  DEAL_STATUS_LABEL,
) as readonly DealStatus[];

export function dealStatusFilter(
  value: string | undefined,
): DealStatus | undefined {
  return DEAL_STATUS_VALUES.find((admitted) => admitted === value);
}

export function dealStatusLabel(value: string): MessageKey | undefined {
  const status = dealStatusFilter(value);
  return status && labelFor(DEAL_STATUS_LABEL, status);
}

// Won reads success, lost reads danger, an open deal carries no status tone.
export function dealStatusTone(
  status: string,
): "success" | "danger" | undefined {
  if (status === "won") {
    return "success";
  }
  if (status === "lost") {
    return "danger";
  }
  return undefined;
}
