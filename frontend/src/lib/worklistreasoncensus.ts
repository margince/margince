// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which reasons this build can put a sentence to.
//
// Its own file because it is a CENSUS: one entry per reason the server may
// send, and the whole point is that adding a reason means adding a line here.
// Kept beside the copy that renders them, it read as incidental to the
// rendering rather than as the list it is.
//
// backend/gates/worklistreasonkinds_test.go holds KNOWN_REASONS equal to the
// contract's WorklistReason.kind enum, and each kind to an English sentence.

import type { WorklistReason } from "./worklisttypes";

// Every reason this client has a sentence for.
//
// A newer server sending a reason this build does not know must not print
// `worklist.because.customer_escalated` at a reader — a missing translation
// returns its own key, so an unrecognised value has to be caught here rather
// than discovered on screen.
export const KNOWN_REASONS = {
  pinned: true,
  reminder_due: true,
  buyer_wrote_last: true,
  non_buyer_request: true,
  waiting_days: true,
  overdue: true,
  due_today: true,
  closing_soon: true,
  expected_revenue: true,
  material: true,
  below_material: true,
  quiet_days: true,
  no_champion: true,
  champion_unknown: true,
  promised: true,
  approved_and_failed: true,
  blocks_customer_work: true,
  account_termination: true,
  routine: true,
  repeated_failure: true,
  legal_deadline: true,
  earlier_requests: true,
  first_asked: true,
  no_next_step: true,
  opened_overdue: true,
  meeting_soon: true,
  meeting_booked: true,
  meeting_unprepared: true,
  response_overdue: true,
  response_due_soon: true,
  unassigned: true,
  stale: true,
  no_reply_history: true,
  asks_nothing: true,
  addressed_elsewhere: true,
  bcc_from_stranger: true,
  link_off_sender: true,
  outcome_unrecorded: true,
  you_wrote_last: true,
  no_reply_days: true,
  met_days_ago: true,
  nothing_sent_since: true,
  no_attendee_on_file: true,
  read_only: true,
} as const;

export type KnownReason = keyof typeof KNOWN_REASONS;

export function known(kind: WorklistReason["kind"]): kind is KnownReason {
  return kind in KNOWN_REASONS;
}

// The valued reasons whose figure is a DAY COUNT rather than a currency
// amount. A count needs the reader's own plural rule ("1 day", not "1 days"),
// which a money figure never does.
const DAYS_VALUED_REASONS = {
  waiting_days: true,
  quiet_days: true,
  earlier_requests: true,
  no_reply_days: true,
  met_days_ago: true,
} as const;

export function daysValued(
  kind: WorklistReason["kind"],
): kind is keyof typeof DAYS_VALUED_REASONS {
  return kind in DAYS_VALUED_REASONS;
}
