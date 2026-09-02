// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { calendarDaysBetween } from "../format/format";

type Person360 = components["schemas"]["Person360"];

// When a contact's silence becomes a reading. The rail's pulse and the move
// card both turn on it, and they turned on two copies of it — one counting
// whole days from the browser's clock, the other counting calendar days from
// the read — which near the boundary disagreed on the same record. One span,
// one count, one clock: the read's own `as_of`, so the two surfaces change
// their word on the same day and hold it while a tab is left open.
export const QUIET_AFTER_DAYS = 14;

/** daysSinceInbound is how long they have been silent, or null if they never wrote. */
export function daysSinceInbound(view: Person360): number | null {
  return view.last_inbound_at
    ? calendarDaysBetween(new Date(view.last_inbound_at), new Date(view.as_of))
    : null;
}

/** isQuiet says whether a silence of `days` has outlasted the span. */
export function isQuiet(days: number): boolean {
  return days > QUIET_AFTER_DAYS;
}
