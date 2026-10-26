import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import {
  daysLate,
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
});

describe("otherTasks", () => {
  it("counts the tasks behind the one the bar draws", () => {
    expect(otherTasks(view(), PROMISE)).toBe(1);
  });

  it("gives no count off a cut page, which would understate the list", () => {
    expect(otherTasks(view({}, true), PROMISE)).toBeUndefined();
  });
});
