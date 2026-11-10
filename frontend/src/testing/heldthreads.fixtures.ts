// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type HeldThread = components["schemas"]["HeldThread"];
type HeldThreadList = components["schemas"]["HeldThreadListResponse"];

// The ledger asks a pending thread at most this often; one asked that often is
// stalled. The server's PendingMaxAttempts.
const stallAttempts = 2;

/**
 * One page of held threads holding `rows`, its counts taken from those rows, as
 * the server answers a list short enough for one page.
 */
export function heldThreadList(
  rows: readonly HeldThread[] = [],
  over: Partial<HeldThreadList> = {},
): HeldThreadList {
  const count = (status: string) =>
    rows.filter((row) => row.status === status).length;
  return {
    data: [...rows],
    page: { next_cursor: null, has_more: false },
    counts: {
      pending: count("pending"),
      unsure: count("unsure"),
      held: count("held"),
      held_by_owner: count("held_by_owner"),
      stalled: rows.filter(
        (row) => row.status === "pending" && row.attempts >= stallAttempts,
      ).length,
    },
    ...over,
  };
}
