// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The server an Overview suite or story meets: a catalog with both presets,
// a sales leader's and an SDR's frame, and evaluations built per case. Its own
// module so the tests and the stories describe one server.

import type { components } from "../../api/schema";
import {
  interval,
  REPORTING_FIXTURE_ZONE,
  reportingMetricNames,
  reportingStoryEvaluation,
  reportingStoryFramework,
} from "../../testing/reporting.fixtures";
import {
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import type { ReportingEvaluation } from "./reporting.model";

type Metric = ReportingEvaluation["metrics"][number];
type MetricID = Metric["id"];
type Definition = components["schemas"]["ReportingMetricDefinition"];
type Comparison = NonNullable<Metric["target_comparison"]>;
type GrantSpec = Parameters<typeof meRoute>[0];

export const SELF = "00000000-0000-4000-8000-0000000000a1";
export const TEAM = {
  kind: "team",
  id: "11111111-1111-4111-8111-111111111111",
  label: "DACH",
} as const;
const MINE = { kind: "owner", id: SELF, label: "Riley Rep" } as const;

const UNITS: Readonly<Record<MetricID, string>> = {
  bookings_won: "money",
  closed_win_rate: "percent",
  open_pipeline: "money",
  stage_age: "days",
  qualified_pipeline_created: "money",
  forecast_landing: "money",
  meetings_held: "count",
  accepted_opportunities: "count",
  leads_created: "count",
  lead_status_moves: "count",
  leads_promoted: "count",
  lead_first_response: "hours",
  leads_awaiting_response: "count",
  partner_pipeline: "money",
  partner_won: "money",
  partner_commission: "money",
  partner_activity: "count",
};

const SALES_METRICS: readonly MetricID[] = [
  "bookings_won",
  "closed_win_rate",
  "open_pipeline",
];
const SDR_METRICS: readonly MetricID[] = [
  "meetings_held",
  "accepted_opportunities",
  "leads_promoted",
  "lead_first_response",
  "leads_awaiting_response",
];

const TEAM_METRICS: readonly MetricID[] = [
  "meetings_held",
  "accepted_opportunities",
  "leads_promoted",
  "leads_awaiting_response",
];

function definition(id: MetricID): Definition {
  return {
    id,
    label: reportingMetricNames[id],
    version: "1",
    unit: UNITS[id],
    temporal_basis: id === "open_pipeline" ? "state_at" : "event_period",
    supports_target: true,
    allowed_filters: ["scope", "period", "target_basis"],
    definition: `${reportingMetricNames[id]}, as the server defines it.`,
    blocks: id === "open_pipeline" ? ["pipeline_movement"] : [],
  };
}

/** The catalog a seat reading both presets is given. */
export const overviewCatalog: components["schemas"]["ReportingCatalog"] = {
  metrics: [...SALES_METRICS, "forecast_landing", ...SDR_METRICS].map((id) =>
    definition(id as MetricID),
  ),
  presets: [
    { id: "sales", metrics: [...SALES_METRICS], blocks: [] },
    { id: "sdr", metrics: [...SDR_METRICS], blocks: [] },
    { id: "team", metrics: [...TEAM_METRICS], blocks: [] },
  ],
};

/** One headline metric, measured unless the case says otherwise. */
export function metric(
  id: MetricID,
  value: number | null,
  over: Partial<Metric> = {},
): Metric {
  return {
    id,
    version: "1",
    unit: UNITS[id] === "money" ? "EUR" : UNITS[id],
    value,
    coverage: { status: "ok", withheld: false },
    evidence: { metric: id, context_id: "interval" },
    ...over,
  };
}

export const compared: Comparison = {
  status: "compared",
  target_metric: "bookings_won",
  basis: "commit_evidence",
  target: 80_000_000,
  landing: 76_000_000,
  won: 42_000_000,
  gap: 4_000_000, // ds:ignore the target comparison money gap, not a CSS gap
  remaining: 38_000_000,
  open_pipeline: 136_000_000,
  coverage: 3.58,
};

export function salesEvaluation(
  over: Readonly<{
    comparison?: Comparison;
    metrics?: Metric[];
  }> = {},
): ReportingEvaluation {
  return {
    ...reportingStoryEvaluation,
    context: {
      ...reportingStoryEvaluation.context,
      scope: TEAM,
      period_kind: "this_quarter",
      target_interval: {
        start_at: "2026-06-30T22:00:00Z",
        end_at: "2026-09-30T22:00:00Z",
      },
    },
    selection: {
      ...reportingStoryEvaluation.selection,
      scope: TEAM,
      period: "this_quarter",
      metrics: [...SALES_METRICS, "forecast_landing"],
      blocks: [],
    },
    metrics: over.metrics ?? [
      metric("bookings_won", 42_000_000),
      metric("closed_win_rate", 31),
      metric("open_pipeline", 136_000_000, {
        coverage: { status: "ok", withheld: false, eligible_count: 18 },
      }),
      metric("forecast_landing", 76_000_000, {
        target_comparison: over.comparison ?? compared,
      }),
    ],
    charts: [],
  };
}

export function prospectingEvaluation(
  metrics: Metric[] = [
    metric("meetings_held", 4, {
      target: 24,
      target_actual: 18,
      attainment: 75,
    }),
    metric("accepted_opportunities", 2, { target: 8, target_actual: 6 }),
    metric("leads_promoted", 3, { denominator: 11 }),
    metric("lead_first_response", 1.7),
    metric("leads_awaiting_response", 3),
  ],
): ReportingEvaluation {
  return {
    ...reportingStoryEvaluation,
    context: {
      ...reportingStoryEvaluation.context,
      scope: MINE,
      period_kind: "this_week",
      interval: {
        start_at: "2026-09-20T22:00:00Z",
        end_at: interval.end_at,
      },
      target_interval: {
        start_at: interval.start_at,
        end_at: "2026-09-30T22:00:00Z",
      },
    },
    selection: {
      ...reportingStoryEvaluation.selection,
      scope: MINE,
      pipeline_id: undefined,
      period: "this_week",
      close_window: "all_open",
      metrics: [...SDR_METRICS],
      blocks: [],
    },
    metrics,
    charts: [],
  };
}

/** The team preset's evaluation for the DACH team this week. */
export function teamEvaluation(): ReportingEvaluation {
  const week = prospectingEvaluation([
    metric("meetings_held", 7),
    metric("accepted_opportunities", 3),
    metric("leads_promoted", 5),
    metric("leads_awaiting_response", 2),
  ]);
  return {
    ...week,
    context: { ...week.context, scope: TEAM },
    selection: { ...week.selection, scope: TEAM, metrics: [...TEAM_METRICS] },
  };
}

type TeamReview = components["schemas"]["SdrTeamReview"];

/** The DACH team's handoff review: two SDRs, one round still waiting. */
export function teamReview(over: Partial<TeamReview> = {}): TeamReview {
  return {
    as_of: interval.end_at,
    scope: TEAM,
    interval: { start_at: "2026-09-20T22:00:00Z", end_at: interval.end_at },
    observed_through: interval.end_at,
    workflow: "handoff",
    withheld: false,
    small_sample_below: 5,
    membership_basis: "current",
    meetings: { held: 7, limited_history: false },
    handoffs: {
      outcomes: { submitted: 6, accepted: 3, rejected: 1, recycled: 1 },
      cohort: {
        rounds: 4,
        accepted: 2,
        rejected: 1,
        recycled: 0,
        waiting: 1,
        median_decision_hours: 20,
      },
      waiting: { waiting: 2, unassigned: 0, blocked: 1, returned: 1 },
      reasons: [
        { outcome: "rejected", label: "Wrong territory", count: 1 },
        { outcome: "recycled", label: "Needs more qualification", count: 1 },
      ],
      unattributed: 0,
    },
    members_withheld: false,
    members: [
      {
        id: SELF,
        label: "Riley Rep",
        meetings_held: 4,
        submitted: 4,
        accepted: 2,
        rejected: 1,
        recycled: 1,
        waiting: 1,
        oldest_waiting_since: "2026-09-21T08:00:00Z",
      },
      {
        id: "00000000-0000-4000-8000-0000000000a2",
        label: "Sam SDR",
        meetings_held: 4,
        submitted: 2,
        accepted: 1,
        rejected: 0,
        recycled: 0,
        waiting: 1,
        oldest_waiting_since: "2026-09-22T08:00:00Z",
      },
    ],
    members_truncated: false,
    ...over,
  };
}

/** The movement read with no second capture yet. */
export function startingMovement(): ReportingEvaluation {
  return {
    ...salesEvaluation(),
    metrics: [metric("open_pipeline", 136_000_000)],
    charts: [
      {
        kind: "pipeline_movement",
        metric: "open_pipeline",
        context_id: "state",
        unit: "EUR",
        coverage: { status: "ok", withheld: false },
        points: [],
        capture_status: {
          last_attempt_at: interval.end_at,
          next_capture_at: "2026-09-23T05:00:00Z",
        },
      },
    ],
  };
}

export type Seat = "leader" | "sdr";

// A day with nothing on it, every source read: the Worklist's own shape.
const QUIET_DAY: components["schemas"]["Worklist"] = {
  as_of: interval.end_at,
  scope: "mine",
  scope_options: ["mine"],
  queue: [],
  summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
  sources_unavailable: [],
  reach: [],
  counts: [],
  readings: {
    changed_since_brief: 0,
    revenue_at_risk_minor: null,
    buyer_replies: 0,
    prospecting: 0,
    review: 0,
    more_available: false,
  },
};

const CONTEXTS: Readonly<Record<Seat, unknown>> = {
  leader: {
    default_scope: TEAM,
    allowed_scopes: [TEAM, MINE],
    capabilities: {
      view_manager_forecast: true,
      submit_manager_forecast: true,
    },
    as_of: interval.end_at,
    timezone: REPORTING_FIXTURE_ZONE,
    base_currency: "EUR",
  },
  sdr: {
    default_scope: MINE,
    allowed_scopes: [MINE],
    capabilities: {
      view_manager_forecast: false,
      submit_manager_forecast: false,
    },
    as_of: interval.end_at,
    timezone: REPORTING_FIXTURE_ZONE,
    base_currency: "EUR",
  },
};

const READS: GrantSpec = {
  report_definition: ["read"],
  reporting_framework: ["read"],
  sales_target: ["read"],
  forecast: ["read"],
  deal: ["read"],
  lead: ["read"],
  pipeline: ["read"],
};

/** Who may change what the Overview's setup notes are about. */
export const ADMIN_GRANTS: GrantSpec = {
  ...READS,
  report_definition: ["read", "create"],
  reporting_framework: ["read", "update"],
  sales_target: ["read", "create", "update"],
};

/**
 * Every read the Overview makes but the evaluation, which the caller answers
 * by its query. `template` is the framework the workspace runs.
 */
export function overviewRoutes(
  over: Readonly<{
    seat?: Seat;
    template?: "sales" | "sdr";
    grants?: GrantSpec;
    summary?: () => Response | Promise<Response>;
    checks?: () => Response | Promise<Response>;
    framework?: () => Response | Promise<Response>;
    team?: () => Response | Promise<Response>;
  }> = {},
): RouteMap {
  const seat = over.seat ?? "leader";
  const template = over.template ?? (seat === "sdr" ? "sdr" : "sales");
  return {
    "GET /me": meRoute(over.grants ?? READS),
    "GET /analytics/context": () => jsonResponse(CONTEXTS[seat]),
    "GET /analytics/metrics": () =>
      jsonResponse({ ...overviewCatalog, schedule_ready: true }),
    "GET /analytics/framework":
      over.framework ??
      (() =>
        jsonResponse({
          ...reportingStoryFramework,
          definition: { ...reportingStoryFramework.definition, template },
        })),
    "GET /pipelines": () =>
      jsonResponse({
        data: [
          {
            id: "sales",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: [],
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    "GET /forecast/assurance/exceptions":
      over.checks ?? (() => jsonResponse({ data: [] })),
    "GET /analytics/sdr-summary":
      over.summary ??
      (() =>
        jsonResponse({
          as_of: interval.end_at,
          scope: MINE,
          workflow: template === "sdr" ? "handoff" : "qualification",
          handoffs:
            template === "sdr"
              ? { waiting: 2, unassigned: 0, blocked: 0, returned: 0 }
              : undefined,
          upcoming_meetings: 5,
        })),
    "GET /analytics/sdr-summary/handoffs": () =>
      jsonResponse({ as_of: interval.end_at, rows: [] }),
    "GET /analytics/sdr-team": over.team ?? (() => jsonResponse(teamReview())),
    "GET /worklist": () => jsonResponse(QUIET_DAY),
  };
}
