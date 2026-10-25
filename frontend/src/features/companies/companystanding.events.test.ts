import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { translate, translatePlural } from "../../i18n";
import { companyTimeline, type TimelineWords } from "./companystanding.events";

type Company360 = components["schemas"]["Company360"];
type Activity = NonNullable<Company360["activities"]>["data"][number];

const WORDS: TimelineWords = {
  t: (key, params) => translate("en", key, params),
  plural: (base, count, params) => translatePlural("en", base, count, params),
  locale: "en",
  zone: UTC_ZONE,
};

const BASE: Company360 = {
  as_of: "2026-08-13T09:00:00Z",
  company: {
    id: "o-1",
    display_name: "Brandt Automotive",
    source: "manual",
    captured_by: "human:test",
    created_at: "2026-06-01T09:00:00Z",
    updated_at: "2026-06-01T09:00:00Z",
  },
  sections_omitted: [],
};

function activity(over: Partial<Activity> & Pick<Activity, "id">): Activity {
  return {
    occurred_precision: "instant",
    kind: "meeting",
    occurred_at: "2026-06-03T10:00:00Z",
    subject: "Kickoff call",
    is_done: true,
    source: "manual",
    captured_by: "human:test",
    created_at: "2026-06-01T09:00:00Z",
    updated_at: "2026-06-01T09:00:00Z",
    ...over,
  };
}

function withActivities(rows: Activity[], over: Partial<Company360> = {}) {
  return {
    ...BASE,
    activities: { data: rows, page: { has_more: false, next_cursor: null } },
    ...over,
  } satisfies Company360;
}

describe("companyTimeline", () => {
  it("draws nothing for an account with no dated rows", () => {
    expect(companyTimeline(BASE, WORDS)).toEqual({ events: [] });
  });

  it("dates what happened and marks the last contact apart", () => {
    const view = withActivities(
      [
        activity({ id: "a-1" }),
        activity({
          id: "a-2",
          kind: "email",
          subject: "Re: retrofit timeline",
          occurred_at: "2026-08-01T08:00:00Z",
        }),
      ],
      {
        last_contact: {
          at: "2026-08-01T08:00:00Z",
          kind: "email",
          activity_id: "a-2",
        },
      },
    );
    const { events } = companyTimeline(view, WORDS);
    expect(events.map((event) => [event.title, event.sub, event.kind])).toEqual(
      [
        ["Kickoff call", "3 Jun · Meeting", "done"],
        ["Re: retrofit timeline", "1 Aug · Last contact", "last"],
      ],
    );
  });

  it("leaves out tasks, meetings that never happened and rows after as_of", () => {
    const view = withActivities([
      activity({ id: "t", kind: "task" }),
      activity({ id: "c", meeting_status: "canceled" }),
      activity({ id: "f", occurred_at: "2026-09-01T10:00:00Z" }),
    ]);
    expect(companyTimeline(view, WORDS).events).toEqual([]);
  });

  it("puts a slipped task, the booked meeting and the close ahead", () => {
    const view = {
      ...BASE,
      next_steps: {
        data: [
          {
            activity_id: "s-1",
            subject: "Renewal paperwork",
            due_at: "2026-07-25T17:00:00Z",
            overdue: true,
          },
        ],
        page: { has_more: false, next_cursor: null },
      },
      next_meeting: {
        activity_id: "m-1",
        starts_at: "2026-08-20T10:00:00Z",
        occurred_precision: "instant",
        subject: "Walkthrough",
        participants: [],
      },
    } satisfies Company360;
    const { events } = companyTimeline(view, WORDS);
    expect(events.map((event) => [event.title, event.sub, event.kind])).toEqual(
      [
        ["Renewal paperwork", "25 Jul · Overdue", "late"],
        ["Walkthrough", "20 Aug · Meeting", "ahead"],
      ],
    );
  });

  it("counts the wait from our last message when they owe the reply", () => {
    const view = {
      ...BASE,
      last_outbound_at: "2026-08-01T08:00:00Z",
      state_strip: {
        account: { lifecycle: "customer", relationship_types: [] },
        engagement: { state: "waiting_on_them" },
      },
    } satisfies Company360;
    const { gap } = companyTimeline(view, WORDS);
    expect(gap).toEqual({
      from: "2026-08-01T08:00:00Z",
      title: "12 days",
      sub: "Waiting on them",
    });
  });
});
