import type { operations } from "../api/schema";
import type { MessageKey } from "../i18n/en";

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
  return status && DEAL_STATUS_LABEL[status];
}
