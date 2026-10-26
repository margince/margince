// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { movesLastActivity } from "./lastactivity";

type Activity = components["schemas"]["Activity"];

const asOf = Date.parse("2026-08-20T12:00:00Z");

function row(fields: Partial<Activity>): Activity {
  return {
    offers_email: false,
    id: "a1",
    kind: "email",
    occurred_at: "2026-08-19T09:00:00Z",
    source: "manual",
    captured_by: "user:1",
    created_at: "2026-08-19T09:00:00Z",
    updated_at: "2026-08-19T09:00:00Z",
    is_done: false,
    ...fields,
  };
}

describe("movesLastActivity", () => {
  it("counts mail, chat, a call, a note and a meeting that took place", () => {
    for (const kind of ["email", "message", "call", "note"] as const) {
      expect(movesLastActivity(row({ kind }), asOf)).toBe(true);
    }
    expect(
      movesLastActivity(row({ kind: "meeting", meeting_status: "held" }), asOf),
    ).toBe(true);
    expect(movesLastActivity(row({ kind: "meeting" }), asOf)).toBe(true);
  });

  it("never counts a task, whoever filed it", () => {
    expect(movesLastActivity(row({ kind: "task" }), asOf)).toBe(false);
  });

  it("never counts a meeting that was called off or nobody came to", () => {
    for (const status of ["canceled", "no_show"] as const) {
      expect(
        movesLastActivity(
          row({ kind: "meeting", meeting_status: status }),
          asOf,
        ),
      ).toBe(false);
    }
  });

  it("never counts what has not happened yet", () => {
    expect(
      movesLastActivity(row({ occurred_at: "2026-08-21T09:00:00Z" }), asOf),
    ).toBe(false);
  });
});
