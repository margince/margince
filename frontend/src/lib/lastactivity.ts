// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Activity = components["schemas"]["Activity"];

// The activity kinds that move a record's last activity, mirrored from
// relstrength.lastActivityKinds. A task is intent, not contact, whoever filed it.
// backend/gates/frontendlastactivitykinds_test.go fails when the two differ.
export const LAST_ACTIVITY_KINDS = [
  "email",
  "message",
  "meeting",
  "call",
  "note",
] as const;

const COUNTING: ReadonlySet<string> = new Set(LAST_ACTIVITY_KINDS);

// A booked meeting counts once it starts; a canceled or no-show one never does.
const COUNTING_MEETING_STATUSES: ReadonlySet<string> = new Set([
  "booked",
  "held",
]);

// Whether a row moves last activity as of `asOf`, as LastActivityCountsSQL
// decides on the server.
export function movesLastActivity(row: Activity, asOf: number): boolean {
  if (!COUNTING.has(row.kind)) return false;
  if (
    row.kind === "meeting" &&
    row.meeting_status != null &&
    !COUNTING_MEETING_STATUSES.has(row.meeting_status)
  ) {
    return false;
  }
  return Date.parse(row.occurred_at) <= asOf;
}
