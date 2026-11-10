import { dayInZone, startOfDayInZone } from "../../format/timezone";
import type {
  ReportingChart,
  ReportingEvaluation,
  ReportingMetricID,
  ReportingSelection,
} from "./reporting.model";
import { useReportingEvaluation } from "./reporting.queries";

const WEEKS = 6;

export type ProspectingOutcome = Extract<
  ReportingMetricID,
  "accepted_opportunities" | "leads_qualified" | "leads_promoted"
>;

export function useProspectingWeeks(
  week: ReportingEvaluation | undefined,
  outcome: ProspectingOutcome | undefined,
) {
  const selection: ReportingSelection | undefined =
    week && outcome
      ? {
          scope: week.selection.scope,
          period: "custom",
          interval: weeksBefore(week),
          target_basis: "month",
          close_window: "all_open",
          metrics: [outcome, "meetings_held"],
          blocks: ["sdr_outcomes"],
        }
      : undefined;
  // A disabled query still needs a key, so it asks for nothing it would send.
  return useReportingEvaluation(
    selection ?? NO_SELECTION,
    selection !== undefined,
  );
}

const NO_SELECTION: ReportingSelection = {
  scope: { kind: "owner" },
  period: "this_week",
  target_basis: "month",
  close_window: "all_open",
  metrics: [],
  blocks: [],
};

function weeksBefore(
  week: ReportingEvaluation,
): ReportingSelection["interval"] {
  const { interval, timezone } = week.context;
  const last = dayInZone(Date.parse(interval.end_at) - 1, timezone);
  const weekday = new Date(`${last}T00:00:00Z`).getUTCDay();
  const fromMonday = (weekday + 6) % 7;
  // Whole weeks: the server stops the week under way at its own reading.
  return {
    start_at: startOfDayInZone(last, timezone, -fromMonday - 7 * (WEEKS - 1)),
    end_at: startOfDayInZone(last, timezone, 7 - fromMonday),
  };
}

export function outcomeWeeks(
  weeks: ReportingEvaluation | undefined,
  outcome: ProspectingOutcome,
): ReportingChart | undefined {
  const chart = weeks?.charts.find(
    (each) => each.kind === "sdr_outcomes" && each.metric === outcome,
  );
  return chart && chart.points.length > 0 ? chart : undefined;
}
