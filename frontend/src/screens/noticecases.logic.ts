// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

export type NoticeCase = components["schemas"]["NoticeCase"];
export type NoticeCaseState = components["schemas"]["NoticeCaseState"];

// The states a duty can rest in, in the order a reader thinks about them:
// owed, then being worked, then the four ways one ends.
//
// Mirrored from the backend's own vocabulary (consent/noticecase.go). It is a
// literal rather than derived from the generated enum because the ORDER is a
// product decision the schema does not carry, and a facet bar that reordered
// itself when somebody added a state would move the control under the reader's
// cursor.
export const NOTICE_STATES = [
  "open",
  "assigned",
  "queued",
  "completed",
  "provided_elsewhere",
  "exempt_with_reason",
  "blocked",
  "not_required",
] as const;

// The states that mean the duty is still owed. Derived from the terminal set
// below rather than listed twice, for the same reason the backend derives it:
// two lists of one vocabulary drift until they disagree about whether a duty is
// open, and the reader who loses is the one looking at a queue that is missing
// a case.
export const UNRESOLVED_NOTICE_STATES: readonly NoticeCaseState[] =
  NOTICE_STATES.filter((state) => !isNoticeResolved(state));

// A duty that has ended, however it ended.
//
// `assigned` is deliberately NOT here. Somebody taking a case has not
// discharged it, and a queue that dropped a claimed duty would leave it owed,
// worked and invisible — the only seat still seeing it would be the one that
// took it, which is exactly the wrong place to put the reminder.
export function isNoticeResolved(state: NoticeCaseState): boolean {
  return (
    state === "completed" ||
    state === "not_required" ||
    state === "provided_elsewhere" ||
    state === "exempt_with_reason"
  );
}

// Whether this duty's deadline has passed.
//
// Overdue is READ from the clock, never stored — the backend refuses to keep an
// `overdue` state for the same reason, because a stored one would leave every
// late case looking on time whenever the sweep that writes it failed to run.
//
// A resolved duty is never overdue: the deadline it met, or missed, stopped
// mattering when the case closed. Marking a closed case red would ask an
// officer to act on something already answered.
export function isNoticeOverdue(
  dueAtIso: string,
  state: NoticeCaseState,
  nowMs: number,
): boolean {
  if (isNoticeResolved(state)) {
    return false;
  }
  return Date.parse(dueAtIso) < nowMs;
}

// Whether this duty can still be claimed.
//
// An ended one cannot, which is what the server says too — assigning a closed
// case would put it back on the queue as work somebody is doing when the duty
// is over.
export function mayAssign(state: NoticeCaseState): boolean {
  return !isNoticeResolved(state);
}

// Whether this duty can be ended without sending anything.
//
// Same rule as claiming, and for a sharper reason: re-excusing a case that was
// genuinely discharged would overwrite a real disclosure with a claim about
// one, and the later note would then read as the reason the first thing
// happened.
export function mayExcuse(state: NoticeCaseState): boolean {
  return !isNoticeResolved(state);
}

// Who is accountable for this duty, as a reader should ask it.
//
// The OWNER, never the state. `assigned` means open-and-claimed, but a blocked
// or queued case can carry an owner too — somebody looking at an obstacle has
// not cleared it, so taking one of those keeps its state and just names the
// seat. And "assigned with no owner" is reachable: owner_user_id is nulled when
// a user is deleted, so a duty claimed by somebody who has since left reads as
// unclaimed, which is the honest answer.
export function noticeOwner(row: NoticeCase): string | null {
  return row.owner_user_id ?? null;
}

// The tone a state should read in.
//
// Only the two that need a reader to look: blocked names an obstacle, and the
// excusing states are closures somebody has to be able to defend. `completed`
// is quiet on purpose — a duty discharged by a delivered disclosure is the
// outcome this queue exists to produce, and colouring it would make the
// ordinary case shout.
export function noticeStateTone(
  state: NoticeCaseState,
): "danger" | "warn" | undefined {
  if (state === "blocked") {
    return "warn";
  }
  if (state === "exempt_with_reason" || state === "provided_elsewhere") {
    return "warn";
  }
  return undefined;
}
