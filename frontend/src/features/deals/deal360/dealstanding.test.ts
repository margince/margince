// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { UTC_ZONE } from "../../../format/timezone";
import { deal, offer } from "../deals.testkit";
import {
  closeTone,
  committeeOf,
  committeeTone,
  dealVerdictTone,
  lastInbound,
  momentumOf,
  momentumTone,
  nextStepOf,
  stageEntries,
  stageTimes,
  standingOffer,
  verdictSourceCount,
} from "./dealstanding";

type Activity = components["schemas"]["Activity"];
type FieldHistoryEntry = components["schemas"]["FieldHistoryEntry"];
type DealStatusCard = components["schemas"]["DealStatusCard"];

const AS_OF = "2026-08-13T12:00:00Z";

function activity(over: Partial<Activity>): Activity {
  return {
    occurred_precision: "instant",
    offers_email: false,
    id: "a",
    kind: "email",
    occurred_at: "2026-08-01T09:00:00Z",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
    ...over,
  };
}

function move(over: Partial<FieldHistoryEntry>): FieldHistoryEntry {
  return {
    id: "h",
    entity_type: "deal",
    entity_id: "d1",
    field: "stage_id",
    changed_at: "2026-06-01T00:00:00Z",
    actor_type: "human",
    actor_id: "u1",
    ...over,
  };
}

describe("the verdict's tone", () => {
  it.each([
    ["live", "success"],
    ["drifting", "warning"],
    ["blocked", "danger"],
    ["cold", "danger"],
    // A word from a newer server is a call this build cannot read, never good
    // news.
    ["stalled_pending_legal", "neutral"],
    [undefined, "neutral"],
  ])("%s reads as %s", (standing, tone) => {
    expect(dealVerdictTone(standing)).toBe(tone);
  });
});

it("counts each cited record once, however many sentences cite it", () => {
  const cite = (entity_id: string) => ({
    entity_type: "activity" as const,
    entity_id,
  });
  const card: DealStatusCard = {
    deal_id: "d1",
    story: { sentences: [] },
    generated_at: AS_OF,
    generated_by: "model",
    verdict: {
      standing: "drifting",
      because: {
        sentences: [
          { text: "One.", evidence: [cite("m1"), cite("m2")] },
          { text: "Two.", evidence: [cite("m1")] },
        ],
      },
    },
  };
  expect(verdictSourceCount(card)).toBe(2);
});

describe("momentum", () => {
  it.each([
    [{ stalled: true, last_activity_at: "2026-08-01" }, "stalled", "danger"],
    [{ stalled: false, wait_until: "2026-09-01" }, "on_hold", "neutral"],
    [
      {
        stalled: false,
        wait_until: "2026-08-20",
        last_activity_at: "2026-08-11",
      },
      "active",
      "success",
    ],
    [{ stalled: false, wait_until: "2026-08-15" }, "quiet", "neutral"],
    [{ stalled: false, last_activity_at: "2026-08-11" }, "active", "success"],
    [{ stalled: false }, "quiet", "neutral"],
  ] as const)("%o reads %s on 20 Aug", (fields, state, tone) => {
    const momentum = momentumOf(
      fields,
      UTC_ZONE,
      new Date("2026-08-20T12:00:00Z"),
    );
    expect(momentum.state).toBe(state);
    expect(momentumTone(momentum)).toBe(tone);
  });
});

describe("the committee", () => {
  const seat = (contact_id: string, engaged = true) => ({
    contact_id,
    role: "champion",
    engaged,
  });
  const coverage = (
    stakeholders: ReturnType<typeof seat>[],
    gapKinds: ("coverage_gap" | "going_cold")[] = [],
  ) => ({
    deal_id: "d1",
    stakeholders,
    our_side: [],
    risks: gapKinds.map((kind) => ({ kind, summary: "" })),
    sections_omitted: [],
  });

  it("names nobody it was not shown", () => {
    expect(committeeOf(coverage([seat("c1")]), true)).toEqual({
      state: "unknown",
    });
    expect(committeeTone({ state: "unknown" })).toBe("neutral");
  });

  it("counts the coverage gaps, not every risk", () => {
    const committee = committeeOf(
      coverage([seat("c1"), seat("c2", false)], ["coverage_gap", "going_cold"]),
      false,
    );
    expect(committee).toEqual({ state: "read", named: 2, engaged: 1, gaps: 1 });
    expect(committeeTone(committee)).toBe("warning");
  });

  it.each([
    [0, 0, "danger"],
    [1, 0, "danger"],
    [2, 0, "success"],
    [3, 1, "warning"],
  ])("%i named with %i gaps reads %s", (named, gaps, tone) => {
    expect(committeeTone({ state: "read", named, engaged: named, gaps })).toBe(
      tone,
    );
  });
});

describe("the close date's tone", () => {
  it.each([
    [{ status: "open" as const }, undefined, "neutral"],
    [
      { status: "open" as const, expected_close_date: "2026-09-30" },
      48,
      "success",
    ],
    [
      {
        status: "open" as const,
        expected_close_date: "2026-09-30",
        close_date_provisional: true,
      },
      48,
      "warning",
    ],
    [
      { status: "open" as const, expected_close_date: "2026-08-01" },
      -12,
      "danger",
    ],
    [
      { status: "won" as const, expected_close_date: "2026-08-01" },
      undefined,
      "success",
    ],
  ])("%o with %s days left reads %s", (fields, daysLeft, tone) => {
    expect(closeTone(fields, daysLeft)).toBe(tone);
  });
});

describe("time in each stage", () => {
  const stages = [{ id: "s1" }, { id: "s2" }, { id: "s3" }];

  it("runs from each entry to the next, and the current stage to as-of", () => {
    const entries = stageEntries(
      [
        move({
          old_value: null,
          new_value: "s1",
          changed_at: "2026-06-01T00:00:00Z",
        }),
        move({
          old_value: "s1",
          new_value: "s2",
          changed_at: "2026-06-13T00:00:00Z",
        }),
      ],
      deal({ created_at: "2026-06-01T00:00:00Z" }),
      true,
    );
    const times = stageTimes({
      stages,
      currentId: "s2",
      entries,
      asOf: AS_OF,
      zone: "UTC",
    });
    expect(times.get("s1")).toEqual({ days: 12, current: false });
    expect(times.get("s2")).toEqual({ days: 61, current: true });
    expect(times.has("s3")).toBe(false);
  });

  it("dates the stage the first advance left by the deal's creation only when the history is whole", () => {
    const rows = [
      move({
        old_value: "s1",
        new_value: "s2",
        changed_at: "2026-06-13T00:00:00Z",
      }),
    ];
    const created = deal({ created_at: "2026-06-01T00:00:00Z" });
    expect(stageEntries(rows, created, true).get("s1")).toBe(
      "2026-06-01T00:00:00Z",
    );
    expect(stageEntries(rows, created, false).has("s1")).toBe(false);
  });

  it("keeps the latest arrival at a stage visited twice", () => {
    const entries = stageEntries(
      [
        move({
          old_value: "s1",
          new_value: "s2",
          changed_at: "2026-06-10T00:00:00Z",
        }),
        move({
          old_value: "s2",
          new_value: "s1",
          changed_at: "2026-06-20T00:00:00Z",
        }),
      ],
      deal({}),
      false,
    );
    expect(entries.get("s1")).toBe("2026-06-20T00:00:00Z");
  });

  it("says nothing about a stage with no known entry", () => {
    const times = stageTimes({
      stages,
      currentId: "s2",
      entries: new Map(),
      asOf: AS_OF,
      zone: "UTC",
    });
    expect(times.size).toBe(0);
  });
});

describe("the offer that stands", () => {
  it("is the newest revision nobody replaced", () => {
    const offers = [
      offer({ id: "o1", revision: 1, status: "superseded" }),
      offer({ id: "o2", revision: 2, status: "sent" }),
    ];
    expect(standingOffer(offers)?.id).toBe("o2");
  });

  it("is none when every offer was replaced", () => {
    expect(standingOffer([offer({ status: "superseded" })])).toBeUndefined();
    expect(standingOffer(undefined)).toBeUndefined();
  });
});

describe("the next step", () => {
  it("is a booked meeting ahead before any task", () => {
    const step = nextStepOf(
      [
        activity({ id: "t", kind: "task", due_at: "2026-08-14T00:00:00Z" }),
        activity({
          id: "m",
          kind: "meeting",
          meeting_status: "booked",
          occurred_at: "2026-08-20T10:00:00Z",
          subject: "Walkthrough",
        }),
      ],
      AS_OF,
      "UTC",
    );
    expect(step).toEqual({
      kind: "meeting",
      at: "2026-08-20T10:00:00Z",
      precision: "instant",
      subject: "Walkthrough",
    });
  });

  it("falls back to the open task due soonest, late or not", () => {
    const step = nextStepOf(
      [
        activity({ id: "t1", kind: "task", due_at: "2026-08-30T00:00:00Z" }),
        activity({ id: "t2", kind: "task", due_at: "2026-08-02T00:00:00Z" }),
        activity({
          id: "t3",
          kind: "task",
          is_done: true,
          due_at: "2026-07-01T00:00:00Z",
        }),
      ],
      AS_OF,
      "UTC",
    );
    expect(step?.at).toBe("2026-08-02T00:00:00Z");
  });

  it("keeps a date-only meeting on today as the next step after its noon", () => {
    // Noon on the 13th in Bangkok, read at 19:00 there.
    const step = nextStepOf(
      [
        activity({
          id: "m",
          kind: "meeting",
          meeting_status: "booked",
          occurred_at: "2026-08-13T05:00:00Z",
          occurred_precision: "day",
        }),
      ],
      AS_OF,
      "Asia/Bangkok",
    );
    expect(step?.kind).toBe("meeting");
  });

  it("is none when nothing is dated ahead", () => {
    expect(nextStepOf([activity({})], AS_OF, "UTC")).toBeUndefined();
  });
});

const buyer = {
  author: { kind: "contact_record", display_name: "Dana", is_reader: false },
} as const;

it("finds the buyer's newest message, never one of ours", () => {
  const newest = lastInbound([
    activity({
      id: "in1",
      direction: "inbound",
      voice: buyer,
      occurred_at: "2026-07-01T00:00:00Z",
    }),
    activity({
      id: "in2",
      direction: "inbound",
      voice: buyer,
      occurred_at: "2026-08-01T00:00:00Z",
    }),
    activity({
      id: "out",
      direction: "outbound",
      voice: {
        author: { kind: "colleague", display_name: "Rep", is_reader: true },
      },
      occurred_at: "2026-08-05T00:00:00Z",
    }),
  ]);
  expect(newest?.id).toBe("in2");
});

it("passes over an inbound row the server says no human wrote", () => {
  const newest = lastInbound([
    activity({
      id: "human",
      direction: "inbound",
      voice: buyer,
      occurred_at: "2026-07-01T00:00:00Z",
    }),
    activity({
      id: "accepted",
      direction: "inbound",
      occurred_at: "2026-08-01T00:00:00Z",
      automatic_kind: "calendar",
    }),
    activity({
      id: "bounce",
      direction: "inbound",
      occurred_at: "2026-08-02T00:00:00Z",
      automatic_kind: "sending_system",
    }),
  ]);
  expect(newest?.id).toBe("human");
});

// A colleague's invitation and a system notice arrive inbound; neither is the
// buyer speaking.
it("does not count an inbound row a colleague or a machine wrote", () => {
  const newest = lastInbound([
    activity({
      id: "buyer",
      direction: "inbound",
      voice: buyer,
      occurred_at: "2026-07-01T00:00:00Z",
    }),
    activity({
      id: "colleague-invite",
      direction: "inbound",
      voice: {
        author: { kind: "colleague", display_name: "Carla", is_reader: false },
      },
      occurred_at: "2026-08-01T00:00:00Z",
    }),
    activity({
      id: "notice",
      direction: "inbound",
      voice: {
        author: {
          kind: "machine",
          address: "noreply@tool.example",
          is_reader: false,
        },
      },
      occurred_at: "2026-08-02T00:00:00Z",
    }),
    activity({
      id: "withheld",
      direction: "inbound",
      occurred_at: "2026-08-03T00:00:00Z",
    }),
  ]);
  expect(newest?.id).toBe("buyer");
});

// An imported note naming a historical author and an upcoming meeting an
// outsider organised are not the buyer writing.
it("does not count a note or a meeting as the buyer's last word", () => {
  const outsider = {
    author: { kind: "unresolved", display_name: "Old CRM", is_reader: false },
  } as const;
  const newest = lastInbound([
    activity({
      id: "buyer",
      kind: "email",
      direction: "inbound",
      voice: buyer,
      occurred_at: "2026-07-01T00:00:00Z",
    }),
    activity({
      id: "imported-note",
      kind: "note",
      voice: outsider,
      occurred_at: "2026-08-01T00:00:00Z",
    }),
    activity({
      id: "future-meeting",
      kind: "meeting",
      direction: "inbound",
      voice: outsider,
      occurred_at: "2026-12-01T00:00:00Z",
    }),
    activity({
      id: "unknown-sender",
      kind: "email",
      direction: "inbound",
      voice: {
        author: {
          kind: "unresolved",
          address: "x@else.example",
          is_reader: false,
        },
      },
      occurred_at: "2026-07-15T00:00:00Z",
    }),
  ]);
  // A sender not on record who mailed in still counts.
  expect(newest?.id).toBe("unknown-sender");
});
