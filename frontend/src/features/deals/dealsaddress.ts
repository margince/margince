// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { routeHash } from "../../lib/router";

/**
 * The deals list, narrowed to one value of one dial.
 *
 * A filter chip's key is its wire parameter, so each dial named in
 * `ADDRESSABLE_DEAL_DIALS` is an address. A new caller widens that list first.
 *
 * Spelled once, here, because three surfaces ask for it: the pipeline report's
 * stage table, its open-deals-per-company table, and the company record's own
 * lost-deal figures. A second spelling is how one of them comes to point
 * somewhere that answers a different question.
 *
 * The dials this can name are the ones the deals endpoint actually reads:
 * `stage_id`, `company_id`, `status` and `forecast_category` are query
 * parameters on `/deals`, while `currency` and a confirmed-close-date bound are
 * not. A figure grouped by something the endpoint cannot filter on has no
 * address, and the caller draws the number plainly rather than being handed one
 * that opens a wider set — a link that narrows less than the figure promised is
 * worse than a figure that admits it is a dead end.
 *
 * `forecast_category` reads both ways only for the four values the wire's own
 * union holds. `slipped` is derived by the report from a claimed category and a
 * close date, and a deal may carry no category at all. Those two figures have no
 * address and say so by drawing no door. `forecastCategoryFilter` decides, not a
 * second list here that would answer differently.
 *
 * `extra` rather than a single record of dials, so that widening this never
 * moves an existing caller: a signature every call site has to follow is one
 * that breaks in the merge when a branch adds the fourth.
 */
export const ADDRESSABLE_DEAL_DIALS = [
  "stage_id",
  "company_id",
  "status",
  "forecast_category",
] as const;

export type DealDial = (typeof ADDRESSABLE_DEAL_DIALS)[number];

export function dealsFilteredBy(
  param: DealDial,
  value: string,
  extra: Readonly<Partial<Record<DealDial, string>>> = {},
): string {
  const dials = new URLSearchParams({ [param]: value, ...extra });
  return `${routeHash({ screen: "deals" })}?${dials.toString()}`;
}
