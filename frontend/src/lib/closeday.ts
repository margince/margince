// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Deal = components["schemas"]["Deal"];

/**
 * The day a deal closed, or for an open one the day it is expected to.
 *
 * A closed deal's day is `closed_at`, the instant every report files it under.
 * `expected_close_date` is a forecast, so it says nothing once the deal is won
 * or lost, and an imported win can carry no forecast at all.
 */
export function closeDayOf(
  deal: Partial<Pick<Deal, "status" | "closed_at" | "expected_close_date">>,
): string | null | undefined {
  const closed = deal.status === "won" || deal.status === "lost";
  return closed ? deal.closed_at : deal.expected_close_date;
}
