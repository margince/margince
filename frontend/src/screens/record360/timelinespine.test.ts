// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { timelineSpineSource } from "./timelinespine";

type Activity = components["schemas"]["Activity"];

const AS_OF = "2026-08-21T09:00:00Z";

function row(extra: Partial<Activity>): Activity {
  return {
    id: "a",
    kind: "email",
    occurred_at: "2026-08-01T09:00:00Z",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
    ...extra,
  };
}

describe("timelineSpineSource", () => {
  it("dates each direction off its newest row, whatever order the page is in", () => {
    const source = timelineSpineSource(
      [
        row({
          id: "1",
          direction: "outbound",
          occurred_at: "2026-08-01T09:00:00Z",
        }),
        row({
          id: "2",
          direction: "inbound",
          occurred_at: "2026-08-05T09:00:00Z",
        }),
        row({
          id: "3",
          direction: "outbound",
          occurred_at: "2026-08-03T09:00:00Z",
        }),
        // A note carries no direction and dates neither side.
        row({ id: "4", kind: "note", occurred_at: "2026-08-20T09:00:00Z" }),
      ],
      AS_OF,
      false,
    );
    expect(source.last_inbound_at).toBe("2026-08-05T09:00:00Z");
    expect(source.last_outbound_at).toBe("2026-08-03T09:00:00Z");
  });

  it("turns the open tasks into the thread's dated stops, late ones marked", () => {
    const source = timelineSpineSource(
      [
        row({
          id: "t1",
          kind: "task",
          subject: "Send the breakdown",
          due_at: "2026-08-10T09:00:00Z",
        }),
        row({
          id: "t2",
          kind: "task",
          subject: "Book the kickoff",
          due_at: "2026-09-08T09:00:00Z",
        }),
        row({ id: "t3", kind: "task", subject: "Done already", is_done: true }),
      ],
      AS_OF,
      false,
    );
    expect(source.next_steps?.data).toEqual([
      {
        activity_id: "t1",
        subject: "Send the breakdown",
        due_at: "2026-08-10T09:00:00Z",
        overdue: true,
      },
      {
        activity_id: "t2",
        subject: "Book the kickoff",
        due_at: "2026-09-08T09:00:00Z",
        overdue: false,
      },
    ]);
  });

  it("carries the cut page as a floor and the reading's own instant", () => {
    const source = timelineSpineSource([], AS_OF, true);
    expect(source.activities?.page?.has_more).toBe(true);
    expect(source.as_of).toBe(AS_OF);
  });
});
