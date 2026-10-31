export const REPORTING_FIXTURE_ZONE = "Europe/Berlin";

import type { components } from "../api/schema";
import type {
  ReportingEvaluation,
  ReportingReport,
} from "../features/analytics/reporting.model";

// The English names the server's catalog gives each metric.
export const reportingMetricNames: Record<
  components["schemas"]["ReportingMetricID"],
  string
> = {
  bookings_won: "Sales won",
  closed_win_rate: "Closed win rate",
  open_pipeline: "Open pipeline",
  stage_age: "Time in current stage",
  qualified_pipeline_created: "Qualified pipeline created",
  meetings_held: "Meetings held",
  accepted_opportunities: "Accepted handoffs",
  forecast_landing: "Forecast landing",
  leads_created: "New leads",
  lead_status_moves: "Lead status changes",
  leads_promoted: "Leads qualified",
  lead_first_response: "First response time",
  leads_awaiting_response: "Leads still waiting for a reply",
  partner_pipeline: "Partner pipeline",
  partner_won: "Partner sales won",
  partner_commission: "Partner commission",
  partner_activity: "Active partners",
  stage_conversion: "Stage conversion",
  lead_cohort_outcomes: "What became of new leads",
};

export const reportingStoryScope = {
  kind: "team",
  id: "11111111-1111-4111-8111-111111111111",
  label: "DACH",
} satisfies components["schemas"]["ReportingScope"];
const coverage = {
  status: "ok",
  withheld: false,
} satisfies components["schemas"]["ReportingCoverage"];
export const interval = {
  start_at: "2026-08-31T22:00:00Z",
  end_at: "2026-09-22T12:00:00Z",
};
export const reportingStoryEvaluation: ReportingEvaluation = {
  evaluation_key: "fixture-evaluation",
  context: {
    scope: reportingStoryScope,
    pipeline_id: "sales",
    population_fingerprint: "fixture-dach",
    interval,
    target_interval: {
      start_at: interval.start_at,
      end_at: "2026-09-30T22:00:00Z",
    },
    period_kind: "this_month",
    timezone: REPORTING_FIXTURE_ZONE,
    currency: "EUR",
    evaluated_at: interval.end_at,
    state_at: interval.end_at,
    framework_revision: 1,
    definition_version: "1",
    member_ids: ["maya", "sam"],
  },
  selection: {
    scope: reportingStoryScope,
    pipeline_id: "sales",
    period: "this_month",
    target_basis: "month",
    close_window: "fiscal_quarter",
    metrics: ["bookings_won", "open_pipeline", "stage_age"],
    blocks: [
      "bookings_trend",
      "stage_distribution",
      "owner_attainment",
      "stage_age",
    ],
  },
  metrics: [
    {
      id: "bookings_won",
      version: "2",
      unit: "EUR",
      value: 21600000,
      target: 30000000,
      target_actual: 21600000,
      attainment: 72,
      coverage,
      evidence: { metric: "bookings_won", context_id: "interval" },
    },
    {
      id: "open_pipeline",
      version: "1",
      unit: "EUR",
      value: 88000000,
      coverage,
      evidence: { metric: "open_pipeline", context_id: "state" },
    },
    {
      id: "stage_age",
      version: "1",
      unit: "days",
      value: 21,
      coverage,
      evidence: { metric: "stage_age", context_id: "state" },
    },
  ],
  charts: [
    {
      kind: "bookings_trend",
      metric: "bookings_won",
      context_id: "interval",
      unit: "EUR",
      coverage,
      interval,
      points: [
        {
          key: "1",
          label: "1 Sep",
          value: 0,
          comparison: 0,
          target: 30000000,
          status: "ok",
          evidence: {
            metric: "bookings_won",
            context_id: "interval",
            through: "2026-09-02T00:00:00Z",
          },
        },
        {
          key: "8",
          label: "8 Sep",
          value: 6400000,
          comparison: 5200000,
          target: 30000000,
          status: "ok",
          evidence: {
            metric: "bookings_won",
            context_id: "interval",
            through: "2026-09-09T00:00:00Z",
          },
        },
        {
          key: "15",
          label: "15 Sep",
          value: 14200000,
          comparison: 10100000,
          target: 30000000,
          status: "ok",
          evidence: {
            metric: "bookings_won",
            context_id: "interval",
            through: "2026-09-16T00:00:00Z",
          },
        },
        {
          key: "22",
          label: "22 Sep",
          value: 21600000,
          comparison: 17280000,
          target: 30000000,
          status: "ok",
          evidence: {
            metric: "bookings_won",
            context_id: "interval",
            through: interval.end_at,
          },
        },
      ],
    },
    {
      kind: "stage_distribution",
      metric: "open_pipeline",
      context_id: "state",
      unit: "EUR",
      coverage,
      state_at: interval.end_at,
      points: [
        {
          key: "discovery",
          label: "Discovery",
          value: 24000000,
          status: "ok",
          evidence: {
            metric: "open_pipeline",
            context_id: "state",
            group_key: "stage:discovery",
          },
        },
        {
          key: "proposal",
          label: "Proposal",
          value: 36000000,
          status: "ok",
          evidence: {
            metric: "open_pipeline",
            context_id: "state",
            group_key: "stage:proposal",
          },
        },
        {
          key: "negotiation",
          label: "Negotiation",
          value: 28000000,
          status: "ok",
          evidence: {
            metric: "open_pipeline",
            context_id: "state",
            group_key: "stage:negotiation",
          },
        },
      ],
    },
    {
      kind: "owner_attainment",
      metric: "bookings_won",
      context_id: "target",
      unit: "EUR",
      coverage,
      interval,
      points: [
        {
          key: "maya",
          label: "Maya",
          value: 12000000,
          target: 15000000,
          status: "ok",
          evidence: {
            metric: "bookings_won",
            context_id: "target",
            group_key: "owner:maya",
          },
        },
        {
          key: "sam",
          label: "Sam",
          value: 9600000,
          target: 15000000,
          status: "ok",
          evidence: {
            metric: "bookings_won",
            context_id: "target",
            group_key: "owner:sam",
          },
        },
      ],
    },
    {
      kind: "stage_age",
      metric: "stage_age",
      context_id: "state",
      unit: "days",
      coverage,
      state_at: interval.end_at,
      points: [
        {
          key: "discovery",
          label: "Discovery",
          value: 8,
          upper: 13,
          observations: 12,
          status: "ok",
          evidence: {
            metric: "stage_age",
            context_id: "state",
            group_key: "stage:discovery",
          },
        },
        {
          key: "proposal",
          label: "Proposal",
          value: 21,
          upper: 27,
          observations: 16,
          status: "ok",
          evidence: {
            metric: "stage_age",
            context_id: "state",
            group_key: "stage:proposal",
          },
        },
        {
          key: "negotiation",
          label: "Negotiation",
          value: 31,
          upper: 44,
          observations: 9,
          status: "ok",
          evidence: {
            metric: "stage_age",
            context_id: "state",
            group_key: "stage:negotiation",
          },
        },
      ],
    },
  ],
};
export const reportingStoryReport: ReportingReport = {
  can_manage: true,
  edition_count: 8,
  cadence: "weekly",
  next_due_at: "2026-09-28T07:00:00Z",
  latest_captured_at: "2026-09-21T07:00:00Z",
  last_status: "succeeded",
  id: "report",
  owner_id: "maya",
  name: "DACH operating review",
  audience: "team",
  audience_team_id: reportingStoryScope.id,
  revision: 3,
  version: 3,
  created_at: interval.start_at,
  selection: reportingStoryEvaluation.selection,
};
export const reportingStoryFramework: components["schemas"]["ReportingFramework"] =
  {
    revision: 1,
    version: 1,
    effective_at: interval.start_at,
    definition: {
      template: "sales",
      qualification: [],
      capture_contexts: [],
      reason: "Fixture framework",
    },
  };
// The filters each figure takes, as the server's catalog lists them.
const PERIOD_FILTERS = ["scope", "period", "target_basis"];
const DEAL_FILTERS = [...PERIOD_FILTERS, "pipeline", "close_window"];
export const reportingStoryCatalog: components["schemas"]["ReportingCatalog"] =
  {
    metrics: [
      ...reportingStoryEvaluation.metrics.map(
        (metric): components["schemas"]["ReportingMetricDefinition"] => ({
          id: metric.id,
          label: reportingMetricNames[metric.id],
          version: metric.version,
          unit:
            metric.unit === reportingStoryEvaluation.context.currency
              ? "money"
              : metric.unit,
          temporal_basis:
            metric.id === "bookings_won" ? "event_period" : "state_at",
          supports_target: metric.id === "bookings_won",
          allowed_filters: DEAL_FILTERS,
          definition:
            metric.id === "bookings_won"
              ? "Won deals grouped by their current owner, in the selected close interval."
              : "Current authorized observations at the capture time.",
          blocks: reportingStoryEvaluation.charts
            .filter((chart) => chart.metric === metric.id)
            .map((chart) => chart.kind),
        }),
      ),
      {
        id: "qualified_pipeline_created",
        label: reportingMetricNames.qualified_pipeline_created,
        version: "1",
        unit: "money",
        temporal_basis: "event_period",
        supports_target: true,
        allowed_filters: DEAL_FILTERS,
        definition:
          "First valid entry into the stage mapping effective at the transition, credited to that event’s owner and valuation.",
        blocks: ["owner_attainment", "target_progress", "metric_reading"],
      },
      {
        id: "meetings_held",
        label: reportingMetricNames.meetings_held,
        version: "2",
        unit: "count",
        temporal_basis: "event_period",
        supports_target: true,
        allowed_filters: PERIOD_FILTERS,
        definition:
          "Eligible customer meetings scheduled in the interval and confirmed held by capture time, credited to the recorded host.",
        blocks: ["sdr_outcomes", "target_progress", "metric_reading"],
      },
      {
        id: "accepted_opportunities",
        label: reportingMetricNames.accepted_opportunities,
        version: "1",
        unit: "count",
        temporal_basis: "event_period",
        supports_target: true,
        allowed_filters: PERIOD_FILTERS,
        definition:
          "Distinct opportunities credited to the originating SDR at their first accepted handoff.",
        blocks: ["sdr_outcomes", "target_progress", "metric_reading"],
      },
    ],
    // The presets the server publishes to a reader of these metrics alone.
    presets: [
      {
        id: "sales",
        metrics: [
          "bookings_won",
          "open_pipeline",
          "stage_age",
          "qualified_pipeline_created",
        ],
        blocks: [
          "bookings_trend",
          "stage_distribution",
          "owner_attainment",
          "stage_age",
        ],
      },
      {
        id: "sdr",
        metrics: ["meetings_held", "accepted_opportunities"],
        blocks: ["sdr_outcomes", "target_progress"],
      },
    ],
  };

/**
 * A reporting period as the server frames it in REPORTING_FIXTURE_ZONE: the
 * read instant, the actuals window and the target window. Berlin leaves summer
 * time at 01:00Z on 25 Oct 2026, so a window across that day has an hour more.
 */
export type ReportingFixtureFrame = Readonly<{
  clock: string;
  periodKind: components["schemas"]["ReportingSelection"]["period"];
  interval: components["schemas"]["ReportingWindow"];
  targetInterval: components["schemas"]["ReportingWindow"];
  fiscalYearStartMonth: number;
}>;

export const reportingFixtureFrames = {
  // Calendar Q4 read on its first month's last day: actuals stop on 31 Oct
  // while the target runs to 31 Dec, across the summer-time change.
  quarterCutoff: {
    clock: "2026-10-31T16:00:00Z",
    periodKind: "this_quarter",
    interval: {
      start_at: "2026-09-30T22:00:00Z",
      end_at: "2026-10-31T16:00:00Z",
    },
    targetInterval: {
      start_at: "2026-09-30T22:00:00Z",
      end_at: "2026-12-31T23:00:00Z",
    },
    fiscalYearStartMonth: 1,
  },
  // A business year from February: Q3 runs 1 Aug to 31 Oct and opens in
  // summer time, then closes in winter time.
  fiscalQuarter: {
    clock: "2026-10-20T10:00:00Z",
    periodKind: "this_quarter",
    interval: {
      start_at: "2026-07-31T22:00:00Z",
      end_at: "2026-10-20T10:00:00Z",
    },
    targetInterval: {
      start_at: "2026-07-31T22:00:00Z",
      end_at: "2026-10-31T23:00:00Z",
    },
    fiscalYearStartMonth: 2,
  },
  // The week from Monday 26 Oct crosses into November, while its targets
  // stay October's.
  weekAcrossMonths: {
    clock: "2026-10-29T09:00:00Z",
    periodKind: "this_week",
    interval: {
      start_at: "2026-10-25T23:00:00Z",
      end_at: "2026-10-29T09:00:00Z",
    },
    targetInterval: {
      start_at: "2026-09-30T22:00:00Z",
      end_at: "2026-10-31T23:00:00Z",
    },
    fiscalYearStartMonth: 1,
  },
  // The week of 19 Oct holds the summer-time change, so it is 169 hours long.
  weekWithClockChange: {
    clock: "2026-10-25T18:00:00Z",
    periodKind: "this_week",
    interval: {
      start_at: "2026-10-18T22:00:00Z",
      end_at: "2026-10-25T18:00:00Z",
    },
    targetInterval: {
      start_at: "2026-09-30T22:00:00Z",
      end_at: "2026-10-31T23:00:00Z",
    },
    fiscalYearStartMonth: 1,
  },
} as const satisfies Record<string, ReportingFixtureFrame>;
