// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The records the named Overview scenarios are built from: evaluations framed
// on a fixed clock, Worklist days, input-check findings and handoff rows.
// Every name and amount is invented.

import type { components } from "../../api/schema";
import {
  type ReportingFixtureFrame,
  reportingFixtureFrames,
  reportingStoryEvaluation,
} from "../../testing/reporting.fixtures";
import {
  metric,
  prospectingEvaluation,
  QUIET_DAY,
  SELF,
  salesEvaluation,
  startingMovement,
  teamEvaluation,
  teamReview,
} from "./analytics.overview.fixtures";
import type { ReportingEvaluation } from "./reporting.model";

type Chart = ReportingEvaluation["charts"][number];
type Point = Chart["points"][number];
type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];
type InputCheck = components["schemas"]["InputCheck"];
type WaitingHandoff = components["schemas"]["SdrWaitingHandoff"];

export const frames = reportingFixtureFrames;

/** An evaluation moved onto a frame's clock, period and fiscal year. */
export function framed(
  evaluation: ReportingEvaluation,
  frame: ReportingFixtureFrame,
): ReportingEvaluation {
  return {
    ...evaluation,
    context: {
      ...evaluation.context,
      period_kind: frame.periodKind,
      interval: frame.interval,
      target_interval: frame.targetInterval,
      evaluated_at: frame.clock,
      state_at: frame.clock,
      fiscal_year_start_month: frame.fiscalYearStartMonth,
    },
    selection: { ...evaluation.selection, period: frame.periodKind },
  };
}

/** Running sales won, one point per labelled day, read through each day. */
export function bookingsTrend(
  frame: ReportingFixtureFrame,
  points: readonly (readonly [label: string, through: string, won: number])[],
  over: Partial<Chart> = {},
): Chart {
  return {
    kind: "bookings_trend",
    metric: "bookings_won",
    context_id: "interval",
    unit: "EUR",
    coverage: { status: "ok", withheld: false },
    interval: frame.interval,
    points: points.map(
      ([label, through, won]): Point => ({
        key: through,
        label,
        value: won,
        status: "ok",
        evidence: { metric: "bookings_won", context_id: "interval", through },
      }),
    ),
    ...over,
  };
}

// Q4 to its cutoff, the week of the clock change included.
const QUARTER_TREND = [
  ["4 Oct", "2026-10-04T22:00:00Z", 6_500_000],
  ["11 Oct", "2026-10-11T22:00:00Z", 14_000_000],
  ["18 Oct", "2026-10-18T22:00:00Z", 23_500_000],
  ["25 Oct", "2026-10-25T23:00:00Z", 33_000_000],
  ["31 Oct", "2026-10-31T16:00:00Z", 42_000_000],
] as const;

/** The stage and team charts of the shared story evaluation. */
export function boardCharts(): Chart[] {
  return reportingStoryEvaluation.charts.filter(
    (chart) =>
      chart.kind === "stage_distribution" || chart.kind === "owner_attainment",
  );
}

/** Sales review on the quarter-cutoff clock, with its trend drawn. */
export function quarterSales(
  over: Parameters<typeof salesEvaluation>[0] = {},
): ReportingEvaluation {
  const frame = frames.quarterCutoff;
  return framed(
    salesEvaluation({
      charts: [bookingsTrend(frame, QUARTER_TREND), ...boardCharts()],
      ...over,
    }),
    frame,
  );
}

/** The movement read on the quarter-cutoff clock, before a second capture. */
export function quarterMovement(): ReportingEvaluation {
  return framed(startingMovement(), frames.quarterCutoff);
}

/** My prospecting on a frame, with the given metrics. */
export function prospectingOn(
  frame: ReportingFixtureFrame,
  metrics?: Parameters<typeof prospectingEvaluation>[0],
): ReportingEvaluation {
  return framed(prospectingEvaluation(metrics), frame);
}

/** Team prospecting on a frame. */
export function teamOn(
  frame: ReportingFixtureFrame,
  metrics?: ReportingEvaluation["metrics"],
): ReportingEvaluation {
  const week = teamEvaluation();
  return framed(metrics ? { ...week, metrics } : week, frame);
}

/** A team review for a frame's week, read at its clock. */
export function teamReviewOn(
  frame: ReportingFixtureFrame,
  over: Parameters<typeof teamReview>[0] = {},
) {
  return teamReview({
    as_of: frame.clock,
    interval: frame.interval,
    observed_through: frame.clock,
    ...over,
  });
}

/** Every SDR figure measured as zero for the frame's week. */
export function zeroWeek(): ReportingEvaluation["metrics"] {
  return [
    metric("meetings_held", 0),
    metric("accepted_opportunities", 0),
    metric("leads_promoted", 0, { denominator: 0 }),
    metric("lead_first_response", null, {
      coverage: { status: "no_data", withheld: false },
    }),
    metric("leads_awaiting_response", 0),
  ];
}

const LEADS = {
  nordhafen: "30000000-0000-4000-8000-000000000001",
  kestrel: "30000000-0000-4000-8000-000000000002",
  bramwell: "30000000-0000-4000-8000-000000000003",
  lindqvist: "30000000-0000-4000-8000-000000000004",
  orchard: "30000000-0000-4000-8000-000000000005",
} as const;

const RILEY = { kind: "user", id: SELF, label: "Riley Rep" } as const;

/**
 * Five ranked rows that carry their reason, due time, owner and verb. The last
 * names no due time, which is a real state of a task.
 */
export const workRows: readonly WorklistItem[] = [
  {
    occurred_precision: "instant",
    id: "work-nordhafen-callback",
    source: "task",
    category: "tasks",
    level: 1,
    urgent: true,
    title: "Call Nordhafen Logistik back about the fleet trial",
    subject: { type: "lead", id: LEADS.nordhafen, label: "Nordhafen Logistik" },
    owner: RILEY,
    due_at: "2026-10-27T09:00:00Z",
    overdue: true,
    destination: "today",
    because: [{ kind: "overdue" }],
    consequence: "task_slips",
    primary_action: "complete",
    actions: ["complete", "snooze", "open"],
  },
  {
    occurred_precision: "instant",
    id: "work-kestrel-reply",
    source: "lead_response",
    category: "leads",
    level: 1,
    title: "Kestrel Fleet Services",
    subject: {
      type: "lead",
      id: LEADS.kestrel,
      label: "Kestrel Fleet Services",
    },
    owner: RILEY,
    due_at: "2026-10-29T11:00:00Z",
    destination: "today",
    because: [{ kind: "response_due_soon" }],
    consequence: "buyer_waits",
    primary_action: "open",
    actions: ["open"],
  },
  {
    occurred_precision: "instant",
    id: "work-bramwell-prep",
    source: "meeting",
    category: "meetings",
    level: 2,
    title: "Bramwell Werkstatt · discovery call",
    subject: { type: "lead", id: LEADS.bramwell, label: "Bramwell Werkstatt" },
    owner: RILEY,
    due_at: "2026-10-30T13:00:00Z",
    destination: "today",
    because: [{ kind: "meeting_unprepared" }],
    consequence: "meeting_unprepared",
    primary_action: "open",
    actions: ["open"],
  },
  {
    occurred_precision: "instant",
    id: "work-lindqvist-followup",
    source: "task",
    category: "tasks",
    level: 2,
    title: "Send Lindqvist Handel the retrofit price sheet",
    subject: { type: "lead", id: LEADS.lindqvist, label: "Lindqvist Handel" },
    owner: RILEY,
    due_at: "2026-10-29T16:00:00Z",
    destination: "today",
    because: [{ kind: "due_today" }],
    consequence: "task_slips",
    primary_action: "complete",
    actions: ["complete", "snooze"],
  },
  {
    occurred_precision: "instant",
    id: "work-orchard-note",
    source: "task",
    category: "tasks",
    level: 3,
    title: "Ask Orchard Depot who signs off on workshop tools",
    subject: { type: "lead", id: LEADS.orchard, label: "Orchard Depot" },
    owner: RILEY,
    destination: "today",
    because: [{ kind: "routine" }],
    consequence: "task_slips",
    primary_action: "complete",
    actions: ["complete"],
  },
];

/** The reader's own day on the clock, with these rows and totals. */
export function workday(
  clock: string,
  queue: readonly WorklistItem[],
  over: Partial<Worklist> = {},
): Worklist {
  return {
    ...QUIET_DAY,
    as_of: clock,
    queue: [...queue],
    summary: {
      urgent: queue.filter((item) => item.urgent).length,
      due: queue.filter((item) => item.due_at).length,
      lower_priority: 0,
      total: queue.length,
    },
    ...over,
  };
}

const DEALS = [
  ["Harbourline depot retrofit", 4_800_000, SELF],
  ["Quayside fleet tools", 3_600_000, SELF],
  ["Marlow workshop suite", 2_900_000, "00000000-0000-4000-8000-0000000000a2"],
  ["Fenwick service plan", 2_200_000, "00000000-0000-4000-8000-0000000000a2"],
  ["Ashgrove trial extension", null, SELF],
  ["Tidewater renewal", null, undefined],
  ["Copperfield add-on", null, undefined],
] as const;

/**
 * The overdue review backlog: 24 open findings on 7 deals. Every deal lacks a
 * next step and an economic buyer. The priced ones also passed their close
 * date. The unpriced ones are committed without an amount. Two have no owner.
 */
export function backlogFindings(clock: string): InputCheck[] {
  return DEALS.flatMap(([name, amount, owner], index) => {
    const subjectID = `40000000-0000-4000-8000-00000000000${index + 1}`;
    const types =
      amount === null
        ? ["no_economic_buyer", "commit_unpriced", "no_next_step"]
        : ["no_economic_buyer", "close_past", "no_next_step"];
    if (index < 3) types.push("close_pushed");
    return types.map(
      (type, at): InputCheck => ({
        id: `41000000-0000-4000-8000-${String(index * 10 + at).padStart(12, "0")}`,
        type,
        subject_kind: "deal",
        subject_id: subjectID,
        subject: { type: "deal", id: subjectID, label: name },
        severity: type === "no_economic_buyer" ? "high" : "medium",
        ...(amount === null ? {} : { affected_minor: amount, currency: "EUR" }),
        ...(owner ? { owner_id: owner } : {}),
        status: "open",
        first_seen_at: "2026-10-12T02:00:00Z",
        last_seen_at: clock,
        inputs_changed: false,
      }),
    );
  });
}

/** Handoffs waiting on others: one resubmitted, one on an inactive seat. */
export function waitingHandoffs(clock: string): WaitingHandoff[] {
  return [
    {
      id: "50000000-0000-4000-8000-000000000001",
      waiting_since: "2026-10-26T08:30:00Z",
      resubmitted: false,
      restricted: false,
      subject: {
        type: "lead",
        id: LEADS.kestrel,
        label: "Kestrel Fleet Services",
      },
      submitter_id: SELF,
      recipient: {
        id: "00000000-0000-4000-8000-0000000000b1",
        label: "Jordan Lead",
        active: true,
      },
    },
    {
      id: "50000000-0000-4000-8000-000000000002",
      waiting_since: "2026-10-28T14:00:00Z",
      resubmitted: true,
      restricted: false,
      subject: {
        type: "lead",
        id: LEADS.bramwell,
        label: "Bramwell Werkstatt",
      },
      submitter_id: SELF,
      recipient: {
        id: "00000000-0000-4000-8000-0000000000b2",
        label: "Casey Closer",
        active: false,
      },
    },
    {
      id: "50000000-0000-4000-8000-000000000003",
      waiting_since: clock,
      resubmitted: false,
      restricted: true,
    },
  ];
}
