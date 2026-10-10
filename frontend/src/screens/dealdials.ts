// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { useEntityName } from "./entityref.queries";

// The deals list's enum-valued dials, narrowed to what the wire admits.
//
// A value arrives as a bare string because an address or a SAVED VIEW carries
// it, and either can hold anything: a view written before a value was renamed,
// or a link typed by hand. An unadmitted value selects nothing rather than
// being sent — the request would be refused, and a list that fails to load says
// less than one that simply is not narrowed. Same reading `tag_mode` takes of
// a stored mode its own enum no longer admits.

// The four buckets a deal's own column can hold. `slipped` is the report's
// derivation from a claimed category and a close date, so it is a dimension of
// the forecast tiles and never a filter here.
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

/**
 * What the company chip calls a company nobody picked in this session — one the
 * address named. `undefined` leaves the id, which is all a name the read
 * answered nothing for has to offer; a read still out says so, and one that
 * failed does not print the id as if it were the answer.
 */
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
