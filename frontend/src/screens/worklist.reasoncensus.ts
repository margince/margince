// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which reasons this build can put a sentence to.
//
// Its own file because it is a CENSUS: one entry per reason the server may
// send, and the whole point is that adding a reason means adding a line here.
// Kept beside the copy that renders them, it read as incidental to the
// rendering rather than as the list it is.

import type { WorklistReason } from "./worklist.queries";

// Every reason this client has a sentence for.
//
// A newer server sending a reason this build does not know must not print
// `worklist.because.customer_escalated` at a reader — a missing translation
// returns its own key, so an unrecognised value has to be caught here rather
// than discovered on screen.
export const KNOWN_REASONS = {
  pinned: true,
  buyer_wrote_last: true,
  waiting_days: true,
  overdue: true,
  due_today: true,
  closing_soon: true,
  expected_revenue: true,
  material: true,
  below_material: true,
  quiet_days: true,
  no_champion: true,
  promised: true,
  approved_and_failed: true,
  blocks_customer_work: true,
  routine: true,
  repeated_failure: true,
  legal_deadline: true,
  meeting_soon: true,
  meeting_unprepared: true,
  response_overdue: true,
  response_due_soon: true,
  unassigned: true,
  stale: true,
  no_reply_history: true,
  asks_nothing: true,
  addressed_elsewhere: true,
  outcome_unrecorded: true,
} as const;

export type KnownReason = keyof typeof KNOWN_REASONS;

export function known(kind: WorklistReason["kind"]): kind is KnownReason {
  return kind in KNOWN_REASONS;
}
