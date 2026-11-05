// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { WorklistItem } from "../worklist/worklist.queries";

/**
 * The Home address dial that opens one row of the day, as a click on it does.
 * Mail links each row it lists here (mailcopy.WorkItemFragment).
 */
export const FOCUS_PARAM = "focus";

/** One row's key: its lane and id, since ids repeat across lanes. */
export function focusKey(item: Pick<WorklistItem, "source" | "id">): string {
  return `${item.source}-${item.id}`;
}
