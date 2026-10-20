// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type WorklistItem = components["schemas"]["WorklistItem"];

/**
 * What a set of loaded Worklist cards stands for, in the contract's two units.
 * A folded group is one card and `batch.count` items, so the day's item counts
 * are only ever compared with `items`, never with `cards`.
 */
export type Represented = Readonly<{
  cards: number;
  items: number;
  /** A group was cut at its read limit, so `items` is a floor. */
  floor: boolean;
}>;

export function represented(rows: readonly WorklistItem[]): Represented {
  let items = 0;
  let floor = false;
  for (const row of rows) {
    items += row.batch?.count ?? 1;
    floor ||= row.batch?.at_least === true;
  }
  return { cards: rows.length, items, floor };
}
