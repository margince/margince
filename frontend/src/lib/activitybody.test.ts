// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { browserZone } from "../format/timezone";
import {
  activityRequestBody,
  draftSaysSomething,
  freshDraft,
  NO_ATTENDEES,
} from "./activitybody";

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

it("files a company call on its selected deal beside attendee contacts", () => {
  const zone = browserZone();
  const body = activityRequestBody(
    {
      ...freshDraft(zone, "call"),
      dealFiling: {
        context: { record_type: "company", record_id: "o1", skip: false },
        dealId: "d2",
      },
    },
    "company",
    "o1",
    zone,
    { contacts: [{ id: "p1", name: "Buyer" }], colleagueIds: [] },
  );
  expect(body.links).toEqual([
    { entity_type: "contact", entity_id: "p1" },
    { entity_type: "deal", entity_id: "d2" },
  ]);
  expect(body.deal_filing).toEqual({
    record_type: "company",
    record_id: "o1",
    skip: false,
  });
});

describe("a call's transcript marker", () => {
  it.each([true, false])(
    "sends a transcript only when opted in: %s",
    (asTranscript) => {
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(new Date("2026-01-10T09:00:00Z"));
      const raw = "\nAnna: I will send the offer by Friday.\n";
      const body = activityRequestBody(
        { ...freshDraft("UTC", "call"), body: raw, asTranscript },
        "contact",
        "p1",
        "UTC",
        NO_ATTENDEES,
      );
      if (asTranscript) {
        expect(body.source_system).toBe("transcript");
        expect(body.body).toBe(raw);
      } else {
        expect(body).not.toHaveProperty("source_system");
        expect(body.body).toBe(raw.trim());
      }
    },
  );
});

it.each(["note", "task"] satisfies ("note" | "task")[])(
  "drops a carried transcript marker on %s",
  (kind) => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-01-10T09:00:00Z"));
    const body = activityRequestBody(
      {
        ...freshDraft("UTC", kind),
        asTranscript: true,
        body: "Anna: I will send the offer.",
      },
      "contact",
      "p1",
      "UTC",
      NO_ATTENDEES,
    );
    expect(body).not.toHaveProperty("source_system");
  },
);

describe("a message's transport", () => {
  const send = (kind: "message" | "note") =>
    activityRequestBody(
      { ...freshDraft("UTC", kind), channelProvider: "whatsapp", body: "Hi" },
      "company",
      "o1",
      "UTC",
      NO_ATTENDEES,
    );

  it("names the transport on a message, filed on the record like a note", () => {
    expect(send("message")).toMatchObject({
      kind: "message",
      channel_provider: "whatsapp",
      links: [{ entity_type: "company", entity_id: "o1" }],
    });
  });

  it("leaves a carried transport off any other kind, which the server refuses", () => {
    expect(send("note")).not.toHaveProperty("channel_provider");
  });

  it("counts a message's words as saying what happened, with no subject", () => {
    const message = freshDraft("UTC", "message");
    expect(draftSaysSomething({ ...message, body: "Asked for a quote" })).toBe(
      true,
    );
    expect(draftSaysSomething({ ...message, body: "  " })).toBe(false);
  });
});
