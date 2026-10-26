import type { operations } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type DealStatus = NonNullable<
  NonNullable<operations["listDeals"]["parameters"]["query"]>["status"]
>;

type ForecastFilter = NonNullable<
  NonNullable<
    operations["listDeals"]["parameters"]["query"]
  >["forecast_category"]
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

// Each record is keyed by the contract's own union, so a value the wire adds
// fails the build here until it is listed. `slipped` is not in the forecast
// union: the report derives it from a claimed category and a close date.
const FORECAST_FILTER: Readonly<Record<ForecastFilter, true>> = {
  commit: true,
  best_case: true,
  pipeline: true,
  omitted: true,
};

export function forecastCategoryFilter(
  value: string | undefined,
): ForecastFilter | undefined {
  return Object.keys(FORECAST_FILTER).find(
    (admitted): admitted is ForecastFilter => admitted === value,
  );
}
