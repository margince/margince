import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import type { Translator } from "../../i18n";
import type { ReadingContext } from "./leadstandingparts";
import { leadTimeline } from "./leadstandingtimeline";

type Lead = components["schemas"]["Lead"];
type Activity = components["schemas"]["Activity"];

const t: Translator = (key: string, params?: Record<string, string>) =>
  params ? `${key}:${JSON.stringify(params)}` : key;

const ctx: ReadingContext = {
  t,
  locale: "en",
  zone: UTC_ZONE,
  record: UTC_ZONE,
  asOf: "2026-08-13T10:00:00Z",
};

const lead: Lead = {
  id: "l1",
  full_name: "Jonas Weber",
  status: "engaged",
  score: 72,
  source: "webform",
  captured_by: "human:u1",
  created_at: "2026-08-02T08:00:00Z",
  updated_at: "2026-08-09T08:00:00Z",
  first_response_at: "2026-08-02T10:00:00Z",
};

function row(extra: Partial<Activity>): Activity {
  return {
    occurred_precision: "instant",
    id: "a1",
    kind: "email",
    occurred_at: "2026-08-09T09:00:00Z",
    is_done: true,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-08-09T09:00:00Z",
    updated_at: "2026-08-09T09:00:00Z",
    ...extra,
  };
}

describe("leadTimeline", () => {
  it("draws arrival, our answer, their newest message and a meeting ahead", () => {
    const { events } = leadTimeline(
      lead,
      [
        row({ id: "in", direction: "inbound" }),
        row({
          id: "out",
          direction: "outbound",
          occurred_at: "2026-08-02T10:00:00Z",
        }),
        row({
          id: "demo",
          kind: "meeting",
          meeting_status: "booked",
          subject: "Depot demo",
          occurred_at: "2026-08-15T09:00:00Z",
        }),
      ],
      "Web form",
      ctx,
    );
    expect(events.map((event) => [event.title, event.kind])).toEqual([
      ["Web form", "done"],
      ["lead.timeline.weReplied", "done"],
      ['standing.wrote:{"name":"Jonas Weber"}', "last"],
      ["Depot demo", "ahead"],
    ]);
  });

  // Their message last leaves the move with us; ours last leaves it with them.
  it("counts the silence since the newest exchange and says whose move it is", () => {
    const theirs = leadTimeline(
      lead,
      [row({ direction: "inbound" })],
      "Web form",
      ctx,
    );
    expect(theirs.gap).toEqual({
      from: "2026-08-09T09:00:00Z",
      title: 'contact.strip.days:{"count":"4"}',
      sub: "lead.gap.yourMove",
    });
    const ours = leadTimeline(
      lead,
      [
        row({
          id: "in",
          direction: "inbound",
          occurred_at: "2026-08-05T09:00:00Z",
        }),
        row({ id: "out", direction: "outbound" }),
      ],
      "Web form",
      ctx,
    );
    expect(ours.gap?.sub).toBe("lead.gap.theirMove");
  });

  it("draws no silence for a closed lead, nor for one with no exchange", () => {
    expect(
      leadTimeline(
        { ...lead, archived_at: "2026-08-12T09:00:00Z" },
        [row({ direction: "inbound" })],
        "Web form",
        ctx,
      ).gap,
    ).toBeUndefined();
    expect(leadTimeline(lead, [], "Web form", ctx).gap).toBeUndefined();
  });

  it("does not count a called-off meeting as contact", () => {
    const { events } = leadTimeline(
      lead,
      [
        row({
          kind: "meeting",
          direction: "inbound",
          meeting_status: "canceled",
        }),
      ],
      "Web form",
      ctx,
    );
    expect(events.map((event) => event.key)).toEqual([
      "received",
      "first-response",
    ]);
  });
});
