import type { components } from "../../api/schema";
import type { ReadingTone } from "../../design-system/readingtone";
import { calendarDaysUntil } from "../../format/daysuntil";
import { displayDay } from "../../format/format.dates";
import { daysPast } from "../../format/lateness";
import type { ProjectHealthState } from "./projecthealth.queries";

// What "How it stands" reads off one Project360, as data rather than words:
// the page words it, the tests pin it. Every count is measured to `as_of`,
// never to the reader's clock, so the card says what the payload says.

type Project360 = components["schemas"]["Project360"];

// A user judged the project; the tone is that judgement's, never a rule's.
const HEALTH_TONE: Readonly<Record<ProjectHealthState, ReadingTone>> = {
  on_track: "success",
  at_risk: "warning",
  off_track: "danger",
};

export type ProjectReading =
  | Readonly<{
      kind: "assessed";
      state: ProjectHealthState;
      note?: string;
      assessedAt: string;
      author?: string;
      tone: ReadingTone;
    }>
  | Readonly<{ kind: "none" | "withheld"; tone: "neutral" }>;

/** The verdict: the standing health reading, or its absence. */
export function projectReading(view: Project360): ProjectReading {
  if (view.sections_omitted.includes("health")) {
    return { kind: "withheld", tone: "neutral" };
  }
  const current = view.health?.current;
  if (!current) {
    return { kind: "none", tone: "neutral" };
  }
  return {
    kind: "assessed",
    state: current.state,
    note: current.note?.trim() || undefined,
    assessedAt: current.assessed_at,
    author: current.source_author,
    tone: HEALTH_TONE[current.state],
  };
}

export type CommitmentsReading = Readonly<{
  overdue: number;
  // True when every row the page carries is overdue and more exist, so the
  // overdue count is the least it could be.
  overdueIsFloor: boolean;
  open: number;
  firstOverdue?: string;
  tone: ReadingTone;
}>;

/** Overdue from the capped rows, open from the uncapped rollup when served. */
export function commitmentsReading(
  view: Project360,
): CommitmentsReading | undefined {
  const section = view.commitments;
  if (!section) {
    return undefined;
  }
  const overdueRows = section.data.filter((row) => row.overdue);
  const open = Math.max(
    view.rollups?.open_commitments ?? section.data.length,
    section.data.length,
  );
  return {
    overdue: overdueRows.length,
    overdueIsFloor:
      section.page.has_more && overdueRows.length === section.data.length,
    open,
    firstOverdue: overdueRows[0]?.subject,
    tone: overdueRows.length > 0 ? "danger" : open > 0 ? "success" : "neutral",
  };
}

export type TimeUsed =
  | Readonly<{
      kind: "running";
      day: number;
      total: number;
      endsOn: string;
      tone: ReadingTone;
    }>
  | Readonly<{ kind: "notStarted"; startsOn: string }>
  | Readonly<{ kind: "noDates" }>;

// Past this share of its days a project still open is drawn as tight.
const TIGHT_SHARE = 0.8;

/**
 * Day N of M, both counted inclusively in the record's zone. The start day is
 * day 1 and the target end is day M, so the two numbers share one rule.
 */
export function timeUsed(view: Project360, zone: string): TimeUsed {
  const { started_at: start, target_end_date: end, phase } = view.project;
  if (!start || !end) {
    return { kind: "noDates" };
  }
  const day = 1 - calendarDaysUntil(start, zone, new Date(view.as_of));
  if (day < 1) {
    return { kind: "notStarted", startsOn: start };
  }
  const total = 1 + calendarDaysUntil(end, zone, displayDay(start, zone));
  let tone: ReadingTone = "success";
  if (day > total && phase !== "closed") {
    tone = "danger";
  } else if (day > total * TIGHT_SHARE) {
    tone = "warning";
  }
  return { kind: "running", day, total, endsOn: end, tone };
}

export type DealsReading = Readonly<{
  won: number;
  total: number;
  // More deals exist than the page carries, so both counts are floors.
  isFloor: boolean;
  openName?: string;
  tone: ReadingTone;
}>;

export function dealsReading(view: Project360): DealsReading | undefined {
  const section = view.deals;
  if (!section) {
    return undefined;
  }
  const won = section.data.filter((deal) => deal.status === "won").length;
  const open = section.data.find((deal) => deal.status === "open");
  let tone: ReadingTone = "neutral";
  if (won > 0) {
    tone = "success";
  } else if (open) {
    tone = "warning";
  } else if (section.data.length > 0) {
    tone = "danger";
  }
  return {
    won,
    total: section.data.length,
    isFloor: section.page.has_more,
    openName: open?.name,
    tone,
  };
}

const SECONDS_PER_DAY = 86_400;

export type PhaseTime = Readonly<{ days: number; current: boolean }>;

/** Whole days spent in each phase so far, summed over every visit. */
export function phaseTimes(view: Project360): ReadonlyMap<string, PhaseTime> {
  return new Map(
    (view.phase_history?.phase_durations ?? []).map((duration) => [
      duration.phase,
      {
        days: Math.floor(duration.seconds / SECONDS_PER_DAY),
        current: duration.current,
      },
    ]),
  );
}

export type LastUpdate = Readonly<{ at: string; daysAgo: number }>;

/** The newest filed activity, from the rollup when served, else the row. */
export function lastUpdate(view: Project360): LastUpdate | undefined {
  const at = view.rollups?.last_activity_at ?? view.project.last_activity_at;
  if (!at) {
    return undefined;
  }
  return {
    at,
    daysAgo: daysPast(Date.parse(at), Date.parse(view.as_of)).days,
  };
}

// A week with nothing filed is the silence the timeline draws as a band.
export const QUIET_AFTER_DAYS = 7;

/** The silence before `as_of` on a project still running, when long enough. */
export function quietStretch(view: Project360): LastUpdate | undefined {
  const last = lastUpdate(view);
  if (
    !last ||
    view.project.phase === "closed" ||
    last.daysAgo < QUIET_AFTER_DAYS
  ) {
    return undefined;
  }
  return last;
}

export type NextCommitment = Readonly<{
  activityId: string;
  subject: string;
  dueAt: string | null;
  overdue: boolean;
  daysLate: number;
  assigneeId: string | null;
  assigneeName: string | null;
  // The other open tasks behind it.
  more: number;
}>;

/** The soonest-due open task, which the server already sorts first. */
export function nextCommitment(view: Project360): NextCommitment | undefined {
  const rows = view.commitments?.data ?? [];
  const first = rows[0];
  if (!first) {
    return undefined;
  }
  const open = Math.max(view.rollups?.open_commitments ?? 0, rows.length);
  return {
    activityId: first.activity_id,
    subject: first.subject,
    dueAt: first.due_at,
    overdue: first.overdue,
    daysLate:
      first.overdue && first.due_at
        ? daysPast(Date.parse(first.due_at), Date.parse(view.as_of)).days
        : 0,
    assigneeId: first.assignee_id,
    assigneeName: first.assignee_name,
    more: open - 1,
  };
}
