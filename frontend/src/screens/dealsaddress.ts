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
 * The dials this can name are the ones the deals endpoint actually reads. There
 * is no `status`, no `forecast_category` and no confirmed-close-date parameter,
 * so a figure counting those cannot be given a door — see the issue tracking
 * that gap rather than inventing one here, because a link that narrows nothing
 * is worse than a figure that admits it is a dead end.
 */
export function dealsFilteredBy(param: string, value: string): string {
  return `${routeHash({ screen: "deals" })}?${param}=${encodeURIComponent(value)}`;
}
