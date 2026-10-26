// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import happenedCases from "../../../../backend/internal/shared/kernel/occurredday/testdata/happened.json";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { LAST_ACTIVITY_KINDS, movesLastActivity } from "./lastactivity";

type Activity = components["schemas"]["Activity"];

const asOf = Date.parse("2026-08-20T12:00:00Z");

function row(fields: Partial<Activity>): Activity {
  return {
    offers_email: false,
    id: "a1",
    kind: "email",
    is_done: false,
    occurred_precision: "instant",
    occurred_at: "2026-08-19T09:00:00Z",
    source: "manual",
    captured_by: "user:1",
    created_at: "2026-08-19T09:00:00Z",
    updated_at: "2026-08-19T09:00:00Z",
    ...fields,
  };
}

describe("movesLastActivity", () => {
  it("counts mail, chat, a call, a note and a meeting that took place", () => {
    for (const kind of ["email", "message", "call", "note"] as const) {
      expect(movesLastActivity(row({ kind }), asOf, UTC_ZONE)).toBe(true);
    }
    expect(
      movesLastActivity(
        row({ kind: "meeting", meeting_status: "held" }),
        asOf,
        UTC_ZONE,
      ),
    ).toBe(true);
    expect(movesLastActivity(row({ kind: "meeting" }), asOf, UTC_ZONE)).toBe(
      true,
    );
  });

  it("never counts a task, whoever filed it", () => {
    expect(movesLastActivity(row({ kind: "task" }), asOf, UTC_ZONE)).toBe(
      false,
    );
  });

  it("never counts a meeting that was called off or nobody came to", () => {
    for (const status of ["canceled", "no_show"] as const) {
      expect(
        movesLastActivity(
          row({ kind: "meeting", meeting_status: status }),
          asOf,
          UTC_ZONE,
        ),
      ).toBe(false);
    }
  });

  it("never counts what has not happened yet", () => {
    expect(
      movesLastActivity(
        row({ occurred_at: "2026-08-21T09:00:00Z" }),
        asOf,
        UTC_ZONE,
      ),
    ).toBe(false);
  });

  // Before 12:00 UTC a day entry dated today is stored ahead of the clock.
  it("counts a day note dated today, and a day meeting once its date ends", () => {
    const today = { occurred_at: "2026-08-20T12:00:00Z" } as const;
    const morning = Date.parse("2026-08-20T09:00:00Z");
    const day = { ...today, occurred_precision: "day" } as const;
    expect(
      movesLastActivity(row({ ...day, kind: "note" }), morning, UTC_ZONE),
    ).toBe(true);
    const meeting = row({ ...day, kind: "meeting", meeting_status: "held" });
    expect(movesLastActivity(meeting, morning, UTC_ZONE)).toBe(false);
    expect(
      movesLastActivity(meeting, Date.parse("2026-08-20T23:59:00Z"), UTC_ZONE),
    ).toBe(false);
    expect(
      movesLastActivity(meeting, Date.parse("2026-08-21T00:00:00Z"), UTC_ZONE),
    ).toBe(true);
  });

  // The cases occurredday.Happened and HappenedSQL answer on the server.
  it.each(happenedCases)("$name", (c) => {
    const kind = LAST_ACTIVITY_KINDS.find((counted) => counted === c.kind);
    if (kind === undefined) throw new Error(`${c.kind} moves no last activity`);
    const shared = row({
      kind,
      occurred_at: c.at,
      occurred_precision: c.precision === "day" ? "day" : "instant",
    });
    expect(movesLastActivity(shared, Date.parse(c.now), c.zone)).toBe(
      c.happened,
    );
  });
});
