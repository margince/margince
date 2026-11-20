// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { List } from "./lists.queries";
import { useMe } from "./session";

/** Whether the signed-in reader is known to belong to no team. */
export function useInNoTeam(): boolean {
  return useMe().data?.teams.length === 0;
}

/**
 * Who a new list is shared with: the reader's teams, or only the reader when
 * they are in no team. A team list would then reach nobody else.
 */
export function useNewListSharing(): List["sharing"] {
  return useInNoTeam() ? "private" : "team";
}
