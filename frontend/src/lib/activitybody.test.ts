// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { activityRequestBody, freshDraft, NO_ATTENDEES } from "./activitybody";

afterEach(() => {
  vi.useRealTimers();
});

// The body a backdated note sends, logged at `now` on a record kept in `zone`.
function loggedNote(now: string, zone: string, day: string) {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(now));
  const draft = { ...freshDraft(zone), subject: "Call recap", day };
  return activityRequestBody(draft, "company", "o1", zone, NO_ATTENDEES);
}

// The server reads a `day` value's date in its own offset. UTC noon on the
// picked date names that date whatever the record zone is.
describe("the time a backdated entry sends", () => {
  it("names the picked date in Auckland's summer, a day ahead of UTC", () => {
    // 09:00 on 16 January in Auckland, still the 15th in UTC.
    const body = loggedNote(
      "2026-01-15T20:00:00Z",
      "Pacific/Auckland",
      "2026-01-15",
    );
    expect(body.occurred_at).toBe("2026-01-15T12:00:00Z");
    expect(body.occurred_precision).toBe("day");
  });

  it("names the picked date at 05:00 in Bangkok, while UTC is on the day before", () => {
    // 05:00 on 16 January in Bangkok is 22:00 on the 15th in UTC.
    const body = loggedNote(
      "2026-01-15T22:00:00Z",
      "Asia/Bangkok",
      "2026-01-15",
    );
    expect(body.occurred_at).toBe("2026-01-15T12:00:00Z");
    expect(body.occurred_precision).toBe("day");
  });

  it("names the picked date in Kiritimati, whose noon is on the UTC day before", () => {
    const body = loggedNote(
      "2026-09-20T00:00:00Z",
      "Pacific/Kiritimati",
      "2026-09-15",
    );
    expect(body.occurred_at).toBe("2026-09-15T12:00:00Z");
    expect(body.occurred_precision).toBe("day");
  });

  it("sends the moment of logging for the record zone's today", () => {
    // The 16th in Bangkok and the 15th in UTC: the 16th is Bangkok's today.
    const body = loggedNote(
      "2026-01-15T22:00:00Z",
      "Asia/Bangkok",
      "2026-01-16",
    );
    expect(body.occurred_at).toBe("2026-01-15T22:00:00.000Z");
    expect(body.occurred_precision).toBe("instant");
  });
});

// The server takes `held` only for a start its own clock has passed.
describe("the time a logged meeting sends", () => {
  function loggedMeeting(now: string, zone: string, day: string) {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(now));
    const draft = { ...freshDraft(zone, "meeting"), subject: "Visit", day };
    return activityRequestBody(draft, "contact", "c1", zone, NO_ATTENDEES);
  }

  it("files a meeting dated today a minute before now, as held", () => {
    const body = loggedMeeting("2026-01-15T10:00:00Z", "UTC", "2026-01-15");
    expect(body).toMatchObject({
      occurred_at: "2026-01-15T09:59:00.000Z",
      occurred_precision: "instant",
      meeting_status: "held",
    });
  });

  it("files yesterday's date as a moment in the first minute of today, never as a date not yet over", () => {
    const body = loggedMeeting("2026-01-15T00:00:30Z", "UTC", "2026-01-14");
    expect(body).toMatchObject({
      occurred_at: "2026-01-14T23:59:30.000Z",
      occurred_precision: "instant",
    });
  });

  it("files an earlier date alone once that date is over", () => {
    const body = loggedMeeting("2026-01-15T00:01:30Z", "UTC", "2026-01-14");
    expect(body).toMatchObject({
      occurred_at: "2026-01-14T12:00:00Z",
      occurred_precision: "day",
    });
  });
});
