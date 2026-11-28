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

/** Internal notes take precedence; an empty string keeps a cleared legacy note empty. */
export function meetingNotesText(
  row: Pick<
    Activity,
    | "captured_by"
    | "meeting_notes"
    | "calendar_managed"
    | "invitation_status"
    | "source_system"
    | "body"
  >,
): string {
  return (
    row.meeting_notes ??
    (!row.calendar_managed &&
    !row.invitation_status &&
    !row.source_system &&
    !row.captured_by?.startsWith("connector:")
      ? (row.body ?? "")
      : "")
  );
}
