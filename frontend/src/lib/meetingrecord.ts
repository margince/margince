// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Activity = components["schemas"]["Activity"];

/**
 * A transcript that records another meeting: the pair is one meeting, so a
 * view that lists or counts meetings skips this row. The mirror of the
 * server's meetingoutcome.RecordsMeetingSQL.
 */
export function recordsAnotherMeeting(
  row: Pick<
    Activity,
    "kind" | "source_system" | "source_activity_id" | "meeting_status"
  >,
): boolean {
  return (
    row.kind === "meeting" &&
    row.source_system === "transcript" &&
    row.source_activity_id != null &&
    row.meeting_status == null
  );
}
