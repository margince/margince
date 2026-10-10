// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];
type CountedStage = { count: number };

/**
 * A column's deal count. It is the true count, or the loaded page's while
 * the totals load, so the column never shows a misleading 0.
 */
export function stageCount(
  totals: CountedStage | undefined,
  loaded: number,
): number {
  return totals?.count ?? loaded;
}

/** The board's deal count: the columns' counts added up, so header and columns agree. */
export function boardDealCount(
  stages: readonly Stage[],
  deals: readonly Deal[],
  totals?: ReadonlyMap<string, CountedStage> | null,
): number {
  return stages.reduce(
    (sum, stage) =>
      sum +
      stageCount(
        totals?.get(stage.id),
        deals.filter((deal) => deal.stage_id === stage.id).length,
      ),
    0,
  );
}
