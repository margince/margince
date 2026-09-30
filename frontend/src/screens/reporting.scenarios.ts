import type { components } from "../api/schema";
import {
  REPORTING_FIXTURE_ZONE,
  reportingStoryEvaluation,
  reportingStoryReport,
} from "./reporting.fixtures";
import type { ReportingEdition, ReportingEvaluation } from "./reporting.model";

const coverage = {
  status: "ok",
  withheld: false,
} satisfies components["schemas"]["ReportingCoverage"];
const counts = [8, 12, 10, 4];
export const sdrEvaluation: ReportingEvaluation = {
  ...reportingStoryEvaluation,
  selection: {
    ...reportingStoryEvaluation.selection,
    pipeline_id: undefined,
    metrics: ["meetings_held", "accepted_opportunities"],
    blocks: ["sdr_outcomes", "target_progress"],
  },
  metrics: [
    {
      id: "meetings_held",
      version: "1",
      unit: "count",
      value: 34,
      target: 40,
      target_actual: 34,
      coverage,
      evidence: { metric: "meetings_held", context_id: "interval" },
    },
    {
      id: "accepted_opportunities",
      version: "1",
      unit: "count",
      value: 12,
      target: 16,
      target_actual: 12,
      coverage,
      evidence: { metric: "accepted_opportunities", context_id: "interval" },
    },
  ],
  charts: ["meetings_held", "accepted_opportunities"].flatMap((metric) => {
    const id =
      metric === "meetings_held" ? "meetings_held" : "accepted_opportunities";
    return [
      {
        kind: "sdr_outcomes",
        metric: id,
        context_id: "month",
        unit: "count",
        coverage,
        interval: reportingStoryEvaluation.context.interval,
        points: counts.map((count, i) => ({
          key: `week-${i}`,
          label: `Week ${i + 1}${i === 3 ? " (partial)" : ""}`,
          value: id === "meetings_held" ? count : [3, 4, 3, 2][i],
          status: "ok",
          evidence: { metric: id, context_id: "month", group_key: `week:${i}` },
        })),
      },
      {
        kind: "target_progress",
        metric: id,
        context_id: "target",
        unit: "count",
        coverage,
        points: [
          {
            key: id,
            label: id,
            value: id === "meetings_held" ? 34 : 12,
            target: id === "meetings_held" ? 40 : 16,
            status: "ok",
            evidence: { metric: id, context_id: "target" },
          },
        ],
      },
    ] satisfies ReportingEvaluation["charts"];
  }),
};

export const forecastEvaluation: ReportingEvaluation = {
  ...reportingStoryEvaluation,
  selection: {
    ...reportingStoryEvaluation.selection,
    period: "this_quarter",
    pipeline_id: undefined,
    metrics: ["forecast_landing", "open_pipeline", "stage_age"],
    blocks: [
      "forecast_support",
      "pipeline_movement",
      "stage_distribution",
      "stage_age",
    ],
  },
  charts: [
    {
      kind: "forecast_support",
      metric: "forecast_landing",
      context_id: "forecast",
      unit: "EUR",
      coverage,
      state_at: reportingStoryEvaluation.context.state_at,
      marker: 110000000,
      points: [
        {
          key: "won",
          label: "Already won",
          value: 21600000,
          status: "ok",
          evidence: {
            metric: "forecast_landing",
            context_id: "forecast",
            group_key: "won",
          },
        },
        {
          key: "supported",
          label: "Supported open",
          value: 45000000,
          status: "ok",
          evidence: {
            metric: "forecast_landing",
            context_id: "forecast",
            group_key: "supported",
          },
        },
        {
          key: "upside",
          label: "Additional upside",
          value: 18000000,
          status: "ok",
          evidence: {
            metric: "forecast_landing",
            context_id: "forecast",
            group_key: "upside",
          },
        },
      ],
    },
    {
      kind: "pipeline_movement",
      metric: "open_pipeline",
      context_id: "movement",
      unit: "EUR",
      coverage,
      opening: 76000000,
      closing: 88000000,
      snapshot_id: "capture-new",
      opening_snapshot_id: "capture-old",
      interval: {
        start_at: "2026-09-15T07:00:00Z",
        end_at: "2026-09-22T07:00:00Z",
      },
      state_at: "2026-09-22T07:00:00Z",
      capture_status: {
        last_attempt_at: "2026-09-22T07:00:00Z",
        last_success_at: "2026-09-22T07:00:00Z",
        next_capture_at: "2026-09-23T07:00:00Z",
      },
      points: [
        { key: "new", label: "New", value: 22000000, status: "ok" },
        { key: "won", label: "Won", value: -9000000, status: "ok" },
        { key: "lost", label: "Lost", value: -7000000, status: "ok" },
        { key: "amount", label: "Amount", value: 6000000, status: "ok" },
      ].map((point) => ({
        ...point,
        status: "ok",
        evidence: {
          metric: "open_pipeline",
          context_id: "movement_delta",
          group_key: point.key,
        },
      })),
    },
    ...reportingStoryEvaluation.charts.filter(
      (chart) =>
        chart.kind === "stage_distribution" || chart.kind === "stage_age",
    ),
  ],
};

export const reportingEditions: ReportingEdition[] = [
  {
    id: "edition-september",
    report_id: reportingStoryReport.id,
    report_revision: 3,
    name: reportingStoryReport.name,
    captured_at: "2026-10-01T07:00:00Z",
    intended_due_at: "2026-10-01T07:00:00Z",
    withheld: false,
    redacted: false,
    evaluation: {
      ...reportingStoryEvaluation,
      context: {
        ...reportingStoryEvaluation.context,
        interval: {
          start_at: "2026-09-01T00:00:00Z",
          end_at: "2026-10-01T00:00:00Z",
        },
      },
    },
  },
  {
    id: "edition-august",
    report_id: reportingStoryReport.id,
    report_revision: 3,
    name: reportingStoryReport.name,
    captured_at: "2026-09-01T07:00:00Z",
    intended_due_at: "2026-09-01T07:00:00Z",
    withheld: false,
    redacted: false,
    evaluation: {
      ...reportingStoryEvaluation,
      context: {
        ...reportingStoryEvaluation.context,
        interval: {
          start_at: "2026-08-01T00:00:00Z",
          end_at: "2026-09-01T00:00:00Z",
        },
      },
      metrics: reportingStoryEvaluation.metrics.map((metric) => ({
        ...metric,
        value: metric.value == null ? null : metric.value * 0.8,
      })),
    },
  },
];

const targetInputs: {
  metric: components["schemas"]["ReportingMetricID"];
  value: number;
  unit: string;
}[] = [
  { metric: "bookings_won", value: 30000000, unit: "EUR" },
  { metric: "qualified_pipeline_created", value: 60000000, unit: "EUR" },
  { metric: "meetings_held", value: 40, unit: "count" },
  { metric: "accepted_opportunities", value: 16, unit: "count" },
];
export const reportingTargets: components["schemas"]["ReportingTarget"][] =
  targetInputs.map((input, index) => ({
    id: `target-${index}`,
    revision: 2,
    version: 2,
    created_at: "2026-09-01T00:00:00Z",
    unit: input.unit,
    definition: {
      metric: input.metric,
      value: input.value,
      scope: reportingStoryEvaluation.context.scope,
      period_kind: "month",
      period_start: "2026-09-01",
      reason: "Agreed monthly commitment",
    },
    interval: {
      start_at: "2026-09-01T00:00:00Z",
      end_at: "2026-10-01T00:00:00Z",
    },
    allocated_value: Math.floor(input.value * 0.8),
    allocation_difference: input.value - Math.floor(input.value * 0.8),
  }));

export const reportingSchedule: components["schemas"]["ReportingSchedule"] = {
  id: "schedule",
  report_id: "report",
  owner_id: "maya",
  version: 3,
  timezone: REPORTING_FIXTURE_ZONE,
  next_due_at: "2026-10-31T08:00:00Z",
  last_status: "suspended",
  definition: {
    report_revision: 3,
    frequency: "monthly",
    day: 31,
    local_time: "09:00",
    enabled: false,
  },
};

export const reportingExecutions: components["schemas"]["ReportingExecution"][] =
  [
    {
      id: "run-suspended",
      report_id: "report",
      report_revision: 3,
      status: "suspended",
      attempt: 1,
      intended_due_at: "2026-09-28T07:00:00Z",
      reason:
        "The accountable owner can no longer generate this report. Review their access and schedule.",
    },
    {
      id: "run-failed",
      report_id: "report",
      report_revision: 3,
      status: "failed",
      attempt: 3,
      intended_due_at: "2026-09-21T07:00:00Z",
      reason: "Report generation failed. Retry the run.",
    },
    {
      id: "run-partial",
      report_id: "report",
      report_revision: 3,
      status: "partial",
      attempt: 1,
      intended_due_at: "2026-09-14T07:00:00Z",
      edition_id: "edition-september",
    },
  ];
