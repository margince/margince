// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// My prospecting's own scenario records: its recent weeks, a handoff sent
// back and the meetings booked ahead. Every name is invented.

import type { components } from "../../api/schema";
import type { ReportingFixtureFrame } from "../../testing/reporting.fixtures";
import { prospectingEvaluation, SELF } from "./analytics.overview.fixtures";
import { framed, LEADS } from "./analytics.overview.scenariodata";
import type { ReportingEvaluation } from "./reporting.model";

type Chart = ReportingEvaluation["charts"][number];
type Point = Chart["points"][number];
type WaitingHandoff = components["schemas"]["SdrWaitingHandoff"];

/** True for the request My prospecting makes for its recent weeks. */
export function isWeeks(url: URL): boolean {
  return (
    url.searchParams.get("period") === "custom" &&
    url.searchParams.getAll("blocks").includes("sdr_outcomes")
  );
}

/**
 * Six weeks of an outcome and of held meetings, one bucket per Monday, the
 * last one the week under way. `null` is a week with no reading, not a zero.
 */
export function prospectingWeeks(
  frame: ReportingFixtureFrame,
  outcome: "accepted_opportunities" | "leads_promoted",
  values: readonly (number | null)[],
  held: readonly (number | null)[] = values.map(() => 1),
): ReportingEvaluation {
  const last = Date.parse(frame.interval.start_at);
  const mondays = values.map((_, index) =>
    new Date(last - (values.length - 1 - index) * 7 * 86_400_000).toISOString(),
  );
  const chart = (
    metricID: "accepted_opportunities" | "leads_promoted" | "meetings_held",
    series: readonly (number | null)[],
  ): Chart => ({
    kind: "sdr_outcomes",
    metric: metricID,
    context_id: "interval",
    unit: "count",
    coverage: { status: "ok", withheld: false },
    interval: {
      start_at: mondays[0] ?? frame.interval.start_at,
      end_at: frame.clock,
    },
    points: series.map(
      (value, index): Point => ({
        key: (mondays[index] ?? "").slice(0, 10),
        label: (mondays[index] ?? "").slice(0, 10),
        at: mondays[index],
        value,
        status: "ok",
        evidence: {
          metric: metricID,
          context_id: "interval",
          group_key: `week:${(mondays[index] ?? "").slice(0, 10)}`,
        },
      }),
    ),
  });
  const base = framed(prospectingEvaluation(), frame);
  return {
    ...base,
    context: {
      ...base.context,
      period_kind: "custom",
      interval: {
        start_at: mondays[0] ?? frame.interval.start_at,
        end_at: frame.clock,
      },
    },
    charts: [chart(outcome, values), chart("meetings_held", held)],
  };
}

/** A handoff sent back to the reader, with the reason the recipient gave. */
export function returnedHandoffs(): WaitingHandoff[] {
  return [
    {
      id: "50000000-0000-4000-8000-000000000009",
      waiting_since: "2026-10-27T15:00:00Z",
      resubmitted: false,
      restricted: false,
      subject: {
        type: "lead",
        id: LEADS.lindqvist,
        label: "Lindqvist Transport",
      },
      submitter_id: SELF,
      reason: "No budget this quarter",
    },
  ];
}

/**
 * Booked meetings inside the summary's fourteen days, soonest first. One has
 * a time, one only a day, and one a record the reader cannot open.
 */
export function upcomingMeetings(clock: string) {
  const at = (hours: number) =>
    new Date(Date.parse(clock) + hours * 3_600_000).toISOString();
  return [
    {
      id: "60000000-0000-4000-8000-000000000001",
      starts_at: at(26),
      starts_precision: "instant",
      title: "Discovery call with Orchard Freight",
      restricted: false,
      record: { type: "lead", id: LEADS.orchard, label: "Orchard Freight" },
    },
    {
      id: "60000000-0000-4000-8000-000000000002",
      starts_at: "2026-11-03T12:00:00Z",
      starts_precision: "day",
      title: "Site visit at Nordhafen Logistik",
      restricted: false,
      record: {
        type: "lead",
        id: LEADS.nordhafen,
        label: "Nordhafen Logistik",
      },
    },
    {
      id: "60000000-0000-4000-8000-000000000003",
      starts_at: at(120),
      starts_precision: "instant",
      title: "Follow-up call",
      restricted: true,
    },
  ] as const satisfies readonly components["schemas"]["SdrUpcomingMeeting"][];
}
