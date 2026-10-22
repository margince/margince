// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { operations } from "../api/schema";
import { dealStatusFilter } from "../format/dealstatus";
import { useT } from "../i18n";

export {
  DEAL_STATUS_LABEL,
  DEAL_STATUS_VALUES,
  dealStatusFilter,
  dealStatusLabel,
} from "../format/dealstatus";

import { useEntityName } from "./entityref.queries";

// The deals list's enum-valued dials, narrowed to what the wire admits.
//
// An address or a saved view carries a value as a bare string, and either can
// hold anything. A value gone stale after a rename, or typed by hand, selects
// nothing instead of being sent. The request would be refused, and a list that
// fails to load says less than one that is simply not narrowed.

type DealQuery = NonNullable<operations["listDeals"]["parameters"]["query"]>;
export type DealStatus = NonNullable<DealQuery["status"]>;
type ForecastFilter = NonNullable<DealQuery["forecast_category"]>;

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

const ENUM_DIALS: Readonly<
  Record<string, (value: string | undefined) => string | undefined>
> = { status: dealStatusFilter, forecast_category: forecastCategoryFilter };

// The filters minus any enum dial holding a value the wire does not admit. The
// row a reader sees and the request the list sends then cannot disagree. The
// same object comes back when nothing is dropped.
export function admittedDealFilters(
  filters: Readonly<Record<string, string>>,
): Readonly<Record<string, string>> {
  const stale = Object.entries(ENUM_DIALS).filter(
    ([key, admit]) => key in filters && admit(filters[key]) === undefined,
  );
  if (stale.length === 0) {
    return filters;
  }
  return Object.fromEntries(
    Object.entries(filters).filter(([key]) => !stale.some(([s]) => s === key)),
  );
}

// What the company chip calls a company nobody picked in this session, one the
// address named. A read still out says so. A failed read does not print the id
// as though it were the answer. A company the reader cannot open stays applied
// and is named as withheld, since its id says nothing they can use.
export function useAppliedCompanyName(id: string | undefined) {
  const t = useT();
  const { name, reading } = useEntityName("company", id);
  if (!id || name) {
    return name ?? undefined;
  }
  if (reading === "pending") {
    return t("common.loading");
  }
  return t(
    reading === "failed" ? "ref.nameLoadFailed" : "deal.companyWithheld",
  );
}
