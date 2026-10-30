import type { components } from "../../api/schema";
import {
  interval,
  REPORTING_FIXTURE_ZONE,
  reportingMetricNames,
  reportingStoryCatalog,
  reportingStoryEvaluation,
  reportingStoryFramework,
  reportingStoryReport,
  reportingStoryScope,
} from "../../testing/reporting.fixtures";
import {
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import type { ReportingEvaluation } from "./reporting.model";

export {
  reportingMetricNames,
  reportingStoryCatalog,
  reportingStoryEvaluation,
  reportingStoryFramework,
  reportingStoryReport,
  reportingStoryScope,
} from "../../testing/reporting.fixtures";
export function reportingStoryRoutes(
  evaluation = reportingStoryEvaluation,
  readOnly = false,
): RouteMap {
  return {
    "GET /me": meRoute(
      {
        report_definition: readOnly
          ? ["read"]
          : ["read", "create", "update", "delete"],
        report_edition: readOnly ? ["read"] : ["read", "create"],
        report_schedule: readOnly ? ["read"] : ["read", "create", "update"],
        sales_target: readOnly ? ["read"] : ["read", "create", "update"],
        reporting_framework: readOnly ? ["read"] : ["read", "update"],
        deal: ["read"],
        forecast: ["read"],
        pipeline: ["read"],
      },
      { seat: readOnly ? "read" : "full" },
    ),
    "GET /installation/settings": () =>
      jsonResponse({
        fiscal_year_start_month: 1,
        timezone: REPORTING_FIXTURE_ZONE,
      }),
    "GET /analytics/context": () =>
      jsonResponse({
        default_scope: reportingStoryScope,
        allowed_scopes: [reportingStoryScope],
        timezone: REPORTING_FIXTURE_ZONE,
        base_currency: "EUR",
        as_of: interval.end_at,
        capabilities: {},
      }),
    "GET /analytics/metrics": () =>
      jsonResponse({ ...reportingStoryCatalog, schedule_ready: true }),
    "GET /analytics/framework": () => jsonResponse(reportingStoryFramework),
    "GET /analytics/runs": () =>
      jsonResponse({ data: [], page: { next_cursor: null, has_more: false } }),
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
    "GET /analytics/evaluate": () => jsonResponse(evaluation),
    "GET /analytics/reports": () =>
      jsonResponse({ data: [reportingStoryReport] }),
    "GET /analytics/reports/report": () => jsonResponse(reportingStoryReport),
    "GET /analytics/reports/report/evaluation": () => jsonResponse(evaluation),
    "GET /analytics/reports/report/schedules": () => jsonResponse({ data: [] }),
    "GET /analytics/reports/report/editions": () => jsonResponse({ data: [] }),
    "GET /analytics/reports/report/executions": () =>
      jsonResponse({ data: [] }),
    "GET /analytics/targets": () => jsonResponse({ data: [] }),
    "GET /analytics/evidence": () =>
      jsonResponse({
        context: evaluation.context,
        metric: "bookings_won",
        rows: [
          {
            key: "deal",
            source_type: "deal",
            source_id: "00000000-0000-4000-8000-000000000012",
            label: "Northstar rollout",
            value: 6400000,
            occurred_at: "2026-09-08T10:00:00Z",
            restricted: false,
          },
        ],
        truncated: false,
      }),
  };
}

// One seat's figure for a metric and the targets it holds for it. A null
// actual is a figure that could not be read; `withheld` is one the seat may not
// see in full.
export type SeatFigure = Readonly<{
  actual: number | null;
  targets?: Readonly<Partial<Record<SeatBasis, number>>>;
  withheld?: boolean;
}>;
type SeatBasis = "month" | "fiscal_quarter";

// The windows a September reading's targets cover in the fixture zone.
const SEAT_TARGET_WINDOWS: Readonly<Record<SeatBasis, typeof interval>> = {
  month: { start_at: "2026-08-31T22:00:00Z", end_at: "2026-09-30T22:00:00Z" },
  fiscal_quarter: {
    start_at: "2026-06-30T22:00:00Z",
    end_at: "2026-09-30T22:00:00Z",
  },
};

// `/analytics/evaluate` for one seat, answered per target basis. The route
// table hands a GET handler no address, and the address holds the basis. So
// this wraps the installed fetch and answers that one path.
export function routeSeatEvaluation(
  figures: Readonly<Record<string, SeatFigure>>,
  asked: URL[] = [],
): void {
  const inner = globalThis.fetch;
  const seat = new Map(Object.entries(figures));
  const stub = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://storybook.local",
    );
    if (!url.pathname.endsWith("/analytics/evaluate"))
      return inner(input, init);
    asked.push(url);
    const basis: SeatBasis =
      url.searchParams.get("target_basis") === "fiscal_quarter"
        ? "fiscal_quarter"
        : "month";
    return jsonResponse({
      ...reportingStoryEvaluation,
      context: {
        ...reportingStoryEvaluation.context,
        target_interval: SEAT_TARGET_WINDOWS[basis],
      },
      metrics: url.searchParams
        .getAll("metrics")
        .map((id) => seatMetric(id, seat.get(id) ?? { actual: 0 }, basis)),
    });
  };
  globalThis.fetch = stub as typeof fetch;
}

// One metric as the evaluation answers it. The server applies no target to a
// withheld figure and has no share to give for an unread one.
function seatMetric(id: string, figure: SeatFigure, basis: SeatBasis) {
  const target = figure.withheld ? undefined : figure.targets?.[basis];
  const money = reportingSectionsCatalog.metrics.some(
    (metric) => metric.id === id && metric.unit === "money",
  );
  const read = figure.actual != null;
  return {
    id,
    version: "1",
    unit: money ? reportingStoryEvaluation.context.currency : "count",
    value: figure.actual,
    target_actual: figure.actual,
    ...(target == null
      ? {}
      : {
          target,
          attainment:
            figure.actual != null && target > 0
              ? (figure.actual / target) * 100
              : null,
        }),
    coverage: {
      status: read ? "ok" : "failed",
      withheld: figure.withheld === true,
    },
    evidence: { metric: id, context_id: "interval" },
  };
}

type MetricDefinition = components["schemas"]["ReportingMetricDefinition"];
type EvaluatedMetric = ReportingEvaluation["metrics"][number];
type EvaluatedChart = ReportingEvaluation["charts"][number];

function sectionDefinition(
  id: MetricDefinition["id"],
  unit: string,
  temporal_basis: MetricDefinition["temporal_basis"],
  supports_target: boolean,
  blocks: MetricDefinition["blocks"],
): MetricDefinition {
  return {
    id,
    label: reportingMetricNames[id],
    version: "1",
    unit,
    temporal_basis,
    supports_target,
    // Only the partner deal figures read a pipeline, as the server's catalog says.
    allowed_filters:
      id === "partner_pipeline" || id === "partner_won"
        ? ["scope", "period", "target_basis", "pipeline"]
        : ["scope", "period", "target_basis"],
    definition: `How ${id} is counted.`,
    blocks: [...blocks, "metric_reading"],
  };
}

// The lead and partner metrics as the catalog describes them.
export const reportingSectionMetrics: readonly MetricDefinition[] = [
  sectionDefinition("leads_created", "count", "event_period", false, [
    "breakdown",
  ]),
  sectionDefinition("lead_status_moves", "count", "event_period", false, [
    "breakdown",
  ]),
  sectionDefinition("leads_promoted", "count", "event_period", true, [
    "target_progress",
  ]),
  sectionDefinition("lead_first_response", "hours", "event_period", false, []),
  sectionDefinition("partner_pipeline", "money", "event_period", true, [
    "breakdown",
    "target_progress",
  ]),
  sectionDefinition("partner_won", "money", "event_period", false, [
    "breakdown",
  ]),
  sectionDefinition("partner_commission", "money", "event_period", false, [
    "breakdown",
  ]),
  sectionDefinition("partner_activity", "count", "state_at", false, [
    "breakdown",
  ]),
];

// Every metric a seat with lead, partner and commission access reads.
export const reportingSectionsCatalog: components["schemas"]["ReportingCatalog"] =
  {
    metrics: [...reportingStoryCatalog.metrics, ...reportingSectionMetrics],
    presets: reportingStoryCatalog.presets,
  };

const sectionCoverage = { status: "ok", withheld: false } as const;

function sectionMetric(
  id: EvaluatedMetric["id"],
  unit: string,
  value: number | null,
  more: Partial<EvaluatedMetric> = {},
): EvaluatedMetric {
  const state = id === "partner_activity";
  return {
    id,
    version: "1",
    unit,
    value,
    coverage: sectionCoverage,
    evidence: { metric: id, context_id: state ? "state" : "interval" },
    ...more,
  };
}

// One breakdown chart: a point per group, each opening its own records.
function breakdown(
  metric: EvaluatedChart["metric"],
  unit: string,
  groups: readonly (readonly [string, string, number, number])[],
): EvaluatedChart {
  const state = metric === "partner_activity";
  const context = state ? "state" : "interval";
  return {
    kind: "breakdown",
    metric,
    context_id: context,
    unit,
    coverage: sectionCoverage,
    ...(state
      ? { state_at: interval.end_at }
      : { interval: reportingStoryEvaluation.context.interval }),
    points: groups.map(([key, label, value, observations]) => ({
      key,
      label,
      value,
      observations,
      status: "ok",
      evidence: { metric, context_id: context, group_key: key },
    })),
  };
}

function targetChart(
  metric: EvaluatedChart["metric"],
  unit: string,
  value: number,
  target: number,
): EvaluatedChart {
  return {
    kind: "target_progress",
    metric,
    context_id: "target",
    unit,
    coverage: sectionCoverage,
    points: [
      {
        key: metric,
        label: metric,
        value,
        target,
        status: "ok",
        evidence: { metric, context_id: "target" },
      },
    ],
  };
}

function sectionEvaluation(
  metrics: EvaluatedMetric[],
  charts: EvaluatedChart[],
): ReportingEvaluation {
  return {
    ...reportingStoryEvaluation,
    context: { ...reportingStoryEvaluation.context, pipeline_id: undefined },
    selection: {
      ...reportingStoryEvaluation.selection,
      pipeline_id: undefined,
      metrics: metrics.map((metric) => metric.id),
      blocks: ["breakdown", "target_progress"],
    },
    metrics,
    charts,
  };
}

export const reportingLeadsEvaluation = sectionEvaluation(
  [
    sectionMetric("leads_created", "count", 24),
    sectionMetric("lead_status_moves", "count", 31),
    sectionMetric("leads_promoted", "count", 6, {
      numerator: 6,
      denominator: 24,
      target: 10,
      target_actual: 6,
      attainment: 60,
    }),
    sectionMetric("lead_first_response", "hours", 3.5),
  ],
  [
    breakdown("leads_created", "count", [
      ["origin:web", "Website", 12, 12],
      ["origin:event", "Events", 8, 8],
      ["origin:", "No origin", 4, 4],
    ]),
    breakdown("lead_status_moves", "count", [
      ["status:contacted", "Contacted", 14, 14],
      ["status:engaged", "Engaged", 9, 9],
      ["status:promoted", "Qualified", 6, 6],
      ["status:disqualified", "Disqualified", 2, 2],
    ]),
    targetChart("leads_promoted", "count", 6, 10),
  ],
);

export const reportingPartnersEvaluation = sectionEvaluation(
  [
    sectionMetric("partner_pipeline", "EUR", 4_500_000, {
      target: 6_000_000,
      target_actual: 4_500_000,
      attainment: 75,
    }),
    sectionMetric("partner_won", "EUR", 1_800_000),
    sectionMetric("partner_commission", "EUR", 180_000),
    sectionMetric("partner_activity", "count", 5, {
      numerator: 5,
      denominator: 8,
    }),
  ],
  [
    breakdown("partner_pipeline", "EUR", [
      ["partner:northwind:sourced", "Northwind · Sourced", 3_000_000, 3],
      ["partner:northwind:influenced", "Northwind · Influenced", 1_000_000, 1],
      [
        "partner:hidden:sourced",
        "Partner you cannot see · Sourced",
        500_000,
        1,
      ],
    ]),
    breakdown("partner_won", "EUR", [
      ["partner:northwind:sourced", "Northwind · Sourced", 1_800_000, 2],
    ]),
    breakdown("partner_commission", "EUR", [
      ["partner:northwind", "Northwind", 120_000, 2],
      ["partner:hidden", "Partner you cannot see", 60_000, 1],
    ]),
    breakdown("partner_activity", "count", [
      ["activity:active", "Active", 5, 5],
      ["activity:inactive", "Inactive", 3, 3],
    ]),
    targetChart("partner_pipeline", "EUR", 4_500_000, 6_000_000),
  ],
);

// The routes a lead or partner view reads: the wider catalog and the view's
// own evaluation.
export function reportingSectionRoutes(
  evaluation: ReportingEvaluation,
): RouteMap {
  return {
    ...reportingStoryRoutes(evaluation),
    "GET /analytics/metrics": () =>
      jsonResponse({ ...reportingSectionsCatalog, schedule_ready: true }),
  };
}
