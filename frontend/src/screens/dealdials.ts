// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { useEntityName } from "./entityref.queries";

// The deals list's enum-valued dials, narrowed to what the wire admits.
//
// An address or a saved view carries a value as a bare string, and either can
// hold anything. A value gone stale after a rename, or typed by hand, selects
// nothing instead of being sent. The request would be refused, and a list that
// fails to load says less than one that is simply not narrowed.

// The four buckets a deal's own column can hold. `slipped` is derived by the
// report from a claimed category and a close date, so no filter can name it.
const FORECAST_FILTER_VALUES = [
  "commit",
  "best_case",
  "pipeline",
  "omitted",
] as const;

export function forecastCategoryFilter(
  value: string | undefined,
): (typeof FORECAST_FILTER_VALUES)[number] | undefined {
  return FORECAST_FILTER_VALUES.find((admitted) => admitted === value);
}

export const DEAL_STATUS_VALUES = ["open", "won", "lost"] as const;

export function dealStatusFilter(
  value: string | undefined,
): (typeof DEAL_STATUS_VALUES)[number] | undefined {
  return DEAL_STATUS_VALUES.find((admitted) => admitted === value);
}

// What the company chip calls a company nobody picked in this session, one the
// address named. `undefined` leaves the id. A read still out says so, and a
// failed read does not print the id as though it were the answer.
export function useAppliedCompanyName(id: string | undefined) {
  const t = useT();
  const { name, reading } = useEntityName("company", id);
  if (!id || name) {
    return name ?? undefined;
  }
  if (reading === "pending") {
    return t("common.loading");
  }
  return reading === "failed" ? t("ref.nameLoadFailed") : undefined;
}
