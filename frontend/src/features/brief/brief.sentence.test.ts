import { describe, expect, it } from "vitest";
import type { MessageKey } from "../../i18n/en";
import { en } from "../../i18n/en";
import type { Worklist, WorklistItem } from "../worklist/worklist.queries";
import { briefSentence } from "./brief.sentence";

// The Brief's opening sentence.
//
// What these are about is the one property that makes a composed sentence
// honest: it may say nothing the rows below it do not already say. So the tests
// compare it against those rows rather than against a string.

/** The catalog's own lookup, with the interpolation the screen does. */
function t(key: MessageKey, values?: Record<string, string>): string {
  const raw = en[key];
  if (!values) {
    return raw;
  }
  return Object.entries(values).reduce<string>(
    (text, [name, value]) => text.replaceAll(`{${name}}`, value),
    raw,
  );
}

function item(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "i1",
    source: "customer_waiting",
    level: 1,
    category: "customer_waiting",
    title: "Aster Handel",
    because: [],
    consequence: "buyer_waits",
    actions: ["open"],
    dispositions: [],
    overdue: false,
    ...over,
  };
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: "2026-06-10T06:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    focus: {
      items: queue.slice(0, 6),
      total: queue.length,
      urgent_remaining: 0,
    },
    counts: [],
    reach: [],
    sources_unavailable: [],
    summary: { total: queue.length, urgent: 0, due: 0, lower_priority: 0 },
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: 0,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
  };
}

describe("the agenda summary", () => {
  it("does not claim an unread day is clear", () => {
    expect(briefSentence(undefined, t, "en")).toBeNull();
  });
  it("uses the complete focus total without repeating a card or counting maintenance", () => {
    const rows = day(
      Array.from({ length: 9 }, (_, index) => item({ id: String(index) })),
    );
    expect(briefSentence(rows, t, "en")).toBeNull();
    const complete = day([
      item(),
      item({ id: "two", category: "decisions", source: "approval" }),
    ]);
    expect(briefSentence(complete, t, "en")).toEqual({
      key: "brief.agenda.summary",
      values: { agenda: "1 customer request · 1 decision" },
    });
    expect(
      briefSentence(
        { ...rows, focus: { items: [], total: 0, urgent_remaining: 0 } },
        t,
        "en",
      ),
    ).toBeNull();
  });
  it.each([
    { scope_truncated: true },
    { sources_unavailable: [{ source: "approval", reason: "failed" }] },
    {
      counts: [
        {
          category: "decisions",
          shown: 1,
          considered: 1,
          more_available: true,
        },
      ],
    },
  ] satisfies Partial<Worklist>[])(
    "withholds exact counts when coverage is partial: %j",
    (partial) => {
      expect(
        briefSentence({ ...day([item()]), ...partial }, t, "en"),
      ).toBeNull();
    },
  );
});

it("does not call a partly read empty queue clear", () => {
  const partial = {
    ...day([]),
    sources_unavailable: [{ source: "task", reason: "failed" }],
  } satisfies Worklist;
  expect(briefSentence(partial, t, "en")).toBeNull();
});

it("does not add a redundant empty agenda when only a side read failed", () => {
  const calendarUnread = {
    ...day([]),
    sources_unavailable: [
      { source: "calendar", reason: "failed", contributes_rows: false },
    ],
  } satisfies Worklist;
  expect(briefSentence(calendarUnread, t, "en")).toBeNull();
});
