import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { translate, translatePlural } from "../../i18n";
import {
  daysLate,
  lateness,
  nextBarTask,
  otherTasks,
  promisedTask,
} from "./companynextbar";

type Company360 = components["schemas"]["Company360"];
type Moment = NonNullable<Company360["moment"]>;

const PROMISE = {
  activity_id: "t-promise",
  subject: "Send the renewal paperwork",
  due_at: "2026-07-25T17:00:00Z",
  overdue: true,
};
const OTHER = {
  activity_id: "t-other",
  subject: "Prepare the walkthrough brief",
  due_at: "2026-08-19T17:00:00Z",
  overdue: false,
};

function moment(rule: Moment["rule"], taskId?: string): Moment {
  return {
    claim_key: "k",
    evidence_fingerprint: "f",
    rule,
    headline: "Send Dana the renewal paperwork",
    why_now: "Due 25 Jul",
    confidence: "observed_fact",
    evidence: taskId ? [{ type: "task", id: taskId, label: "Renewal" }] : [],
    recommended_action: {
      kind: "complete_task",
      label: "Send",
      state: "available",
    },
  };
}

function view(over: Partial<Company360> = {}, hasMore = false): Company360 {
  return {
    as_of: "2026-08-13T09:00:00Z",
    company: {
      id: "o-1",
      display_name: "Brandt",
      source: "manual",
      captured_by: "human:test",
      created_at: "2026-06-01T09:00:00Z",
      updated_at: "2026-06-01T09:00:00Z",
    },
    sections_omitted: [],
    next_steps: {
      data: [OTHER, PROMISE],
      page: { has_more: hasMore, next_cursor: null },
    },
    ...over,
  };
}

describe("the Next bar's task", () => {
  it("is the task the moment's promise cites, not the first on the list", () => {
    const drawn = view({ moment: moment("overdue_promise", "t-promise") });
    expect(promisedTask(drawn)?.activity_id).toBe("t-promise");
    expect(nextBarTask(drawn)?.activity_id).toBe("t-promise");
  });

  it("is no task when the moment cites none, so nothing is dated late", () => {
    expect(nextBarTask(view({ moment: moment("gone_quiet") }))).toBeUndefined();
  });

  it("falls back to the first open task on the quiet rung", () => {
    const drawn = view({ moment: moment("nothing_needed", "t-promise") });
    expect(nextBarTask(drawn)?.activity_id).toBe("t-other");
  });
});

describe("daysLate", () => {
  it("counts whole days past the due date at as_of", () => {
    expect(daysLate(PROMISE, "2026-08-13T09:00:00Z", "UTC")).toBe(19);
  });

  it("says nothing for a task that is not overdue", () => {
    expect(daysLate(OTHER, "2026-08-13T09:00:00Z", "UTC")).toBeUndefined();
  });

  it("counts midnights in the reader's zone, not elapsed 24-hour spans", () => {
    const due = { ...PROMISE, due_at: "2026-08-11T21:00:00Z" };
    // 35 hours later: one elapsed day, two midnights in Berlin.
    expect(daysLate(due, "2026-08-13T08:00:00Z", "Europe/Berlin")).toBe(2);
    expect(daysLate(due, "2026-08-13T08:00:00Z", "UTC")).toBe(2);
    // Hours past a date due late tonight in UTC is yesterday in Berlin.
    expect(daysLate(due, "2026-08-11T22:30:00Z", "UTC")).toBe(0);
    expect(daysLate(due, "2026-08-11T22:30:00Z", "Europe/Berlin")).toBe(1);
  });
});

const words = {
  t: (key: Parameters<typeof translate>[1]) => translate("en", key),
  plural: (
    base: Parameters<typeof translatePlural>[1],
    count: number,
    params?: Record<string, string>,
  ) => translatePlural("en", base, count, params),
  locale: "en" as const,
};

describe("lateness", () => {
  it("gives the badge and the sentence the same count across midnight", () => {
    const due = { ...PROMISE, due_at: "2026-08-11T21:00:00Z" };
    const late = daysLate(due, "2026-08-13T08:00:00Z", "Europe/Berlin");
    expect(late).toBeDefined();
    expect(lateness(late ?? -1, words)).toEqual({
      badge: "2 days late",
      sentence: "Due 2 days ago and still open.",
    });
  });

  it("says one day in both places", () => {
    expect(lateness(1, words)).toEqual({
      badge: "1 day late",
      sentence: "Due one day ago and still open.",
    });
  });

  it("shows no badge for a task due earlier today", () => {
    expect(lateness(0, words)).toEqual({
      sentence: "Due earlier today and still open.",
    });
  });
});

describe("otherTasks", () => {
  it("counts the tasks behind the one the bar draws", () => {
    expect(otherTasks(view(), PROMISE)).toBe(1);
  });

  it("gives no count off a cut page, which would understate the list", () => {
    expect(otherTasks(view({}, true), PROMISE)).toBeUndefined();
  });
});
