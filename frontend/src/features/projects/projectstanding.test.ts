import { describe, expect, it } from "vitest";
import { UTC_ZONE } from "../../format/timezone";
import { project, project360 } from "../../testing/projects.fixtures";
import {
  commitmentsReading,
  dealsReading,
  nextCommitment,
  phaseTimes,
  projectReading,
  QUIET_AFTER_DAYS,
  quietStretch,
  timeUsed,
} from "./projectstanding";
import { deliveringProject360 } from "./projectstanding.fixtures";
import { projectMoments } from "./projectstanding.timeline";

// What How it stands reads off a Project360, pinned to the payload's own
// `as_of` so no assertion depends on the day the suite runs.

const AS_OF = "2026-08-13T10:00:00Z";
const page = { next_cursor: null, has_more: false };

function task(
  id: string,
  overrides: Partial<{ due_at: string | null; overdue: boolean }> = {},
) {
  return {
    activity_id: id,
    subject: `Task ${id}`,
    due_at: "2026-07-03T09:00:00Z",
    assignee_id: null,
    assignee_name: null,
    overdue: false,
    ...overrides,
  };
}

function deal(id: string, status: "open" | "won" | "lost") {
  return {
    id,
    name: `Deal ${id}`,
    pipeline_id: "pl",
    stage_id: "s1",
    status,
    closed_at: status === "open" ? null : "2026-06-02T09:00:00Z",
    amount_minor: 450_000,
    currency: "EUR",
    source: "manual",
    captured_by: "u-me",
    created_at: "2026-06-01T09:00:00Z",
    updated_at: "2026-06-02T09:00:00Z",
  };
}

const ASSESSMENT = {
  id: "h1",
  project_id: "pr-1",
  state: "at_risk" as const,
  note: "Feed is late.",
  assessed_at: "2026-08-10T09:00:00Z",
  source: "human",
  source_author: "Lena Fischer",
  created_at: "2026-08-10T09:00:00Z",
  superseded: false,
};

describe("the reading is a user's, never a default", () => {
  it.each([
    ["on_track", "success"],
    ["at_risk", "warning"],
    ["off_track", "danger"],
  ] as const)("%s reads in %s", (state, tone) => {
    const view = project360({
      health: { current: { ...ASSESSMENT, state } },
    });
    expect(projectReading(view)).toMatchObject({ kind: "assessed", tone });
  });

  it("is not assessed when nobody judged it", () => {
    expect(projectReading(project360({ health: {} }))).toEqual({
      kind: "none",
      tone: "neutral",
    });
  });

  it("says withheld when the grant keeps it back", () => {
    const view = project360({ sections_omitted: ["health"] });
    expect(projectReading(view).kind).toBe("withheld");
  });

  it("drops a blank note so the sentence falls back to who and when", () => {
    const view = project360({
      health: { current: { ...ASSESSMENT, note: "  " } },
    });
    expect(projectReading(view)).toMatchObject({ note: undefined });
  });
});

describe("commitments", () => {
  it("counts overdue from the rows and open from the uncapped rollup", () => {
    const view = project360({
      commitments: {
        data: [task("a", { overdue: true }), task("b")],
        page,
      },
    });
    expect(commitmentsReading(view)).toEqual({
      overdue: 1,
      overdueIsFloor: false,
      open: 4,
      firstOverdue: "Task a",
      tone: "danger",
    });
  });

  it("calls the overdue count a floor when every carried row is overdue", () => {
    const view = project360({
      commitments: {
        data: [task("a", { overdue: true })],
        page: { next_cursor: "c", has_more: true },
      },
    });
    expect(commitmentsReading(view)?.overdueIsFloor).toBe(true);
  });

  it("reads none open as neutral, not as success", () => {
    const view = project360({
      rollups: undefined,
      commitments: { data: [], page },
    });
    expect(commitmentsReading(view)?.tone).toBe("neutral");
  });

  it("is absent when the section is withheld", () => {
    expect(commitmentsReading(project360({ commitments: undefined }))).toBe(
      undefined,
    );
  });
});

describe("time used counts both numbers inclusively", () => {
  const dated = (overrides: Parameters<typeof project>[0]) =>
    project360({
      as_of: AS_OF,
      project: project({
        started_at: "2026-03-01",
        target_end_date: "2026-11-30",
        phase: "delivering",
        ...overrides,
      }),
    });

  it("reads Day 166 of 275 on 13 Aug for a 1 Mar to 30 Nov project", () => {
    expect(timeUsed(dated({}), UTC_ZONE)).toEqual({
      kind: "running",
      day: 166,
      total: 275,
      endsOn: "2026-11-30",
      tone: "success",
    });
  });

  it.each([
    ["2026-08-20", "delivering", "warning"],
    ["2026-08-01", "delivering", "danger"],
    ["2026-08-01", "closed", "warning"],
  ] as const)("ending %s while %s reads %s", (target_end_date, phase, tone) => {
    expect(timeUsed(dated({ target_end_date, phase }), UTC_ZONE)).toMatchObject(
      { tone },
    );
  });

  it("says not started before the start day", () => {
    expect(timeUsed(dated({ started_at: "2026-09-01" }), UTC_ZONE)).toEqual({
      kind: "notStarted",
      startsOn: "2026-09-01",
    });
  });

  it("says no dates when either date is missing", () => {
    expect(timeUsed(dated({ target_end_date: null }), UTC_ZONE)).toEqual({
      kind: "noDates",
    });
  });
});

describe("deals", () => {
  it.each([
    [["won", "open"], "success", "Deal b"],
    [["open"], "warning", "Deal a"],
    [["lost"], "danger", undefined],
    [[], "neutral", undefined],
  ] as const)("%j reads %s", (statuses, tone, openName) => {
    const data = statuses.map((status, index) =>
      deal(String.fromCharCode(97 + index), status),
    );
    const reading = dealsReading(project360({ deals: { data, page } }));
    expect(reading).toMatchObject({ tone, openName, total: data.length });
  });
});

describe("the figures and the axis", () => {
  it("gives each phase its whole days", () => {
    expect(phaseTimes(project360()).get("initiative")).toEqual({
      days: 31,
      current: true,
    });
  });

  it(`draws a quiet band only after ${QUIET_AFTER_DAYS} days with nothing filed`, () => {
    const at = (last: string) =>
      quietStretch(
        project360({
          as_of: AS_OF,
          rollups: undefined,
          project: project({ last_activity_at: last }),
        }),
      );
    expect(at("2026-08-06T09:00:00Z")).toEqual({
      at: "2026-08-06T09:00:00Z",
      daysAgo: 7,
    });
    expect(at("2026-08-08T09:00:00Z")).toBeUndefined();
  });

  it("dates the phase moves, the won deal, the late task and the end", () => {
    const view = project360({
      as_of: AS_OF,
      project: project({ target_end_date: "2026-11-30" }),
      deals: { data: [deal("a", "won"), deal("b", "open")], page },
      commitments: { data: [task("t", { overdue: true })], page },
    });
    expect(
      projectMoments(view).map((moment) => [moment.what, moment.kind]),
    ).toEqual([
      ["opened", "done"],
      ["won", "done"],
      ["due", "late"],
      ["update", "last"],
      ["end", "end"],
    ]);
  });

  it("opens the axis on the start day and draws no phase move", () => {
    const moments = projectMoments(deliveringProject360());
    expect(moments.filter((moment) => moment.key.startsWith("phase-"))).toEqual(
      [],
    );
    expect(moments.find((moment) => moment.what === "opened")).toMatchObject({
      at: "2026-03-01",
      kind: "done",
      phase: "initiative",
      by: { actor_type: "human", actor_name: "Lena Fischer" },
    });
  });

  it("names the last update after the newest filed activity", () => {
    const activity = (id: string, subject: string, at: string) => ({
      id,
      kind: "note" as const,
      subject,
      occurred_at: at,
      occurred_precision: "instant" as const,
      is_done: false,
      source: "manual",
      captured_by: "human:u-1",
      created_at: at,
      updated_at: at,
    });
    const view = deliveringProject360({
      activities: {
        data: [
          activity("a-1", "Kickoff call", "2026-06-03T09:00:00Z"),
          activity("a-2", "Site report", "2026-08-06T09:00:00Z"),
        ],
        page,
      },
    });
    expect(
      projectMoments(view).find((moment) => moment.what === "update"),
    ).toMatchObject({ kind: "last", subject: "Site report" });
  });

  it("names the next task, how late it is at as_of, and the rest behind it", () => {
    const view = project360({
      as_of: AS_OF,
      commitments: { data: [task("a", { overdue: true }), task("b")], page },
    });
    expect(nextCommitment(view)).toMatchObject({
      activityId: "a",
      overdue: true,
      daysLate: 41,
      more: 3,
    });
  });
});
