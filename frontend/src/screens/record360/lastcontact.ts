// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// When the thread last had contact, and how long it has waited since.
//
// The thread's head stop is titled "Last contact" and the gap after it reads
// "No contact since". Both read the instant from `lastContactAt`, so the gap
// always starts where the stop before it is dated.

import { CALLED_OFF_MEETING, isExchange } from "./exchangekinds";
import type { SpineSource } from "./spine";

type Row = NonNullable<SpineSource["activities"]>["data"][number];

/**
 * isContactBy says whether a timeline row is contact that took place by
 * `asOf`, the rule both the drawn conversations and `lastContactAt` read.
 * A note or a called-off meeting is not contact (relstrength.InteractionCountsSQL).
 * A booked meeting dated ahead has not happened. Dates compare as instants,
 * since one moment is written with any offset; a malformed date is NaN and fails.
 */
export function isContactBy(row: Row, asOf: number): boolean {
  const at = Date.parse(row.occurred_at ?? "");
  return (
    isExchange(row.kind) &&
    !CALLED_OFF_MEETING.has(row.meeting_status ?? "") &&
    at <= asOf
  );
}

/**
 * lastContactAt is the newest contact in either direction: an email, a call,
 * a chat message or a meeting that took place.
 *
 * It takes the newest of the server's own readings and the rows on the page.
 * The server's `last_contact` covers rows past the loaded page. The rows cover
 * a read that sends no such reading, such as a deal's bare timeline page.
 */
export function lastContactAt(view: SpineSource): string | undefined {
  const asOf = Date.parse(view.as_of);
  const candidates = [
    view.last_contact?.at,
    view.last_outbound_at,
    view.last_inbound_at,
    view.health?.last_meeting_at,
  ];
  for (const row of view.activities?.data ?? []) {
    if (isContactBy(row, asOf)) {
      candidates.push(row.occurred_at);
    }
  }
  let newest: string | undefined;
  for (const at of candidates) {
    if (at && (!newest || Date.parse(at) > Date.parse(newest))) {
      newest = at;
    }
  }
  return newest;
}

/**
 * waitingSince is the instant the gap counts from, or nothing when there is
 * no gap to draw.
 *
 * No gap on an account we never wrote to: a meeting alone is not a message
 * anyone owes a reply to. No gap when their own message is the last contact
 * either, because that is a conversation in progress.
 */
export function waitingSince(view: SpineSource): string | undefined {
  const since = view.last_outbound_at ? lastContactAt(view) : undefined;
  if (
    !since ||
    (view.last_inbound_at &&
      Date.parse(view.last_inbound_at) >= Date.parse(since))
  ) {
    return undefined;
  }
  return since;
}
