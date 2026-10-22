// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { ScheduledSend } from "./scheduledqueue";
import { dayGroups, readQueue, relativeDay } from "./scheduledtimeline";

const ZONE = "Europe/Berlin";

function send(
  id: string,
  at: string,
  status: ScheduledSend["status"] = "scheduled",
): ScheduledSend {
  return {
    id,
    status,
    scheduled_at: at,
    scheduled_tz: ZONE,
    subject: id,
    to: ["a@acme.test"],
    version: 1,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
  };
}

describe("dayGroups", () => {
  it("sorts by send time and breaks days on the reader's wall clock, not UTC", () => {
    const days = dayGroups(
      [
        send("late", "2026-10-21T22:30:00Z"),
        send("early", "2026-10-20T07:00:00Z"),
        send("same-day", "2026-10-20T15:00:00Z"),
      ],
      ZONE,
    );
    // 22:30Z is 00:30 on the 22nd in Berlin (CEST), so it is the next day's.
    expect(days.map((d) => [d.day, d.sends.map((s) => s.id)])).toEqual([
      ["2026-10-20", ["early", "same-day"]],
      ["2026-10-22", ["late"]],
    ]);
  });

  it("returns no days for an empty queue", () => {
    expect(dayGroups([], ZONE)).toEqual([]);
  });
});

describe("relativeDay", () => {
  const now = new Date("2026-10-20T10:00:00Z");

  it("names today and tomorrow and nothing else", () => {
    expect(relativeDay("2026-10-20", now, ZONE)).toBe("today");
    expect(relativeDay("2026-10-21", now, ZONE)).toBe("tomorrow");
    expect(relativeDay("2026-10-22", now, ZONE)).toBeNull();
    expect(relativeDay("2026-10-19", now, ZONE)).toBeNull();
  });

  it("rolls over a month end", () => {
    const eve = new Date("2026-10-31T10:00:00Z");
    expect(relativeDay("2026-11-01", eve, ZONE)).toBe("tomorrow");
  });
});

describe("readQueue", () => {
  const now = new Date("2026-10-20T10:00:00Z");

  it("splits by what the sender must do and names the message that goes next", () => {
    const queue = readQueue(
      [
        send("later", "2026-10-22T09:00:00Z"),
        send("today", "2026-10-20T15:00:00Z"),
        send("stuck", "2026-10-19T09:00:00Z", "held"),
        send("done", "2026-10-18T09:00:00Z", "sent"),
        send("gone", "2026-10-18T09:00:00Z", "cancelled"),
      ],
      now,
      ZONE,
    );
    expect(queue.held.map((s) => s.id)).toEqual(["stuck"]);
    expect(queue.closed.map((s) => s.id)).toEqual(["done", "gone"]);
    expect(queue.goingToday.map((s) => s.id)).toEqual(["today"]);
    expect(queue.next?.id).toBe("today");
    expect(queue.counts).toEqual({ waiting: 2, held: 1 });
  });

  it("has no next message when nothing waits", () => {
    expect(readQueue([], now, ZONE).next).toBeUndefined();
  });
});
