import {
  interval,
  REPORTING_FIXTURE_ZONE,
  reportingStoryCatalog,
  reportingStoryEvaluation,
  reportingStoryFramework,
  reportingStoryReport,
  reportingStoryScope,
} from "./reporting.fixtures";
import { jsonResponse, meRoute, type RouteMap } from "./story-utils";

export {
  reportingStoryCatalog,
  reportingStoryEvaluation,
  reportingStoryFramework,
  reportingStoryReport,
  reportingStoryScope,
} from "./reporting.fixtures";
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
