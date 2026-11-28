// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { meetingNotesText, recordsAnotherMeeting } from "./meetingrecord";

describe("recordsAnotherMeeting", () => {
  const transcript = {
    kind: "meeting" as const,
    source_system: "transcript",
    source_activity_id: "m-1",
    meeting_status: null,
  };

  it("is a meeting transcript that names its meeting and has no outcome", () => {
    expect(recordsAnotherMeeting(transcript)).toBe(true);
  });

  it("is not a transcript that answers a task with its own outcome", () => {
    expect(
      recordsAnotherMeeting({ ...transcript, meeting_status: "held" }),
    ).toBe(false);
  });

  it("is not a meeting with no source, nor typed notes", () => {
    expect(
      recordsAnotherMeeting({ ...transcript, source_activity_id: null }),
    ).toBe(false);
    expect(recordsAnotherMeeting({ ...transcript, source_system: null })).toBe(
      false,
    );
  });
});

describe("internal meeting notes", () => {
  const calendar = {
    body: "Provider agenda",
    meeting_notes: null,
    calendar_managed: true,
    invitation_status: null,
    source_system: "gcal",
    captured_by: "connector:gcal",
  };
  it("keeps provider text out of the notes editor", () =>
    expect(meetingNotesText(calendar)).toBe(""));
  it("keeps earlier notes the API resolved from a rep edit", () =>
    expect(
      meetingNotesText({ ...calendar, meeting_notes: "Earlier rep notes" }),
    ).toBe("Earlier rep notes"));
  it("keeps an explicit clear empty", () =>
    expect(meetingNotesText({ ...calendar, meeting_notes: "" })).toBe(""));
  it("still reads the legacy body of a manual meeting", () =>
    expect(
      meetingNotesText({
        ...calendar,
        calendar_managed: false,
        source_system: null,
        captured_by: "human:rep",
      }),
    ).toBe("Provider agenda"));
});
