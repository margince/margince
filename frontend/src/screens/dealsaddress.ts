// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { routeHash } from "../app/router";

/**
 * The deals list, narrowed to one value of one dial.
 *
 * A figure that counts deals names a set, and the deals list can already be
 * narrowed to exactly that set — a filter chip's key IS its wire parameter
 * (`screens/deals.tsx`'s `dealFilterChips`), so `stage_id` and
 * `organization_id` are addresses rather than something to add.
 *
 * Spelled once, here, because three surfaces ask for it: the pipeline report's
 * stage table, its open-deals-per-company table, and the company record's own
 * lost-deal figures. A second spelling is how one of them comes to point
 * somewhere that answers a different question.
 *
 * The dials this can name are the ones the deals endpoint actually reads:
 * `stage_id`, `organization_id` and `status` are query parameters on `/deals`,
 * while `currency`, `forecast_category` and a confirmed-close-date bound are
 * not. A figure grouped by something the endpoint cannot filter on has no
 * address, and the caller draws the number plainly rather than being handed one
 * that opens a wider set — a link that narrows less than the figure promised is
 * worse than a figure that admits it is a dead end.
 *
 * `extra` rather than a single record of dials, so that widening this never
 * moves an existing caller: a signature every call site has to follow is one
 * that breaks in the merge when a branch adds the fourth.
 */
export function dealsFilteredBy(
  param: string,
  value: string,
  extra: Readonly<Record<string, string>> = {},
): string {
  const dials = new URLSearchParams({ [param]: value, ...extra });
  return `${routeHash({ screen: "deals" })}?${dials.toString()}`;
}
