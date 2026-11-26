// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { recordsAnotherMeeting } from "./meetingrecord";

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
