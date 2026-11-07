import type { components } from "../../api/schema";
import type { AnalyticsContext, AnalyticsScope } from "./analytics.context";
import type { OverviewView } from "./analytics.overview.choice";
import { TEAM_PERIODS } from "./analytics.overview.context";
import type { ReportingSelection } from "./reporting.model";
import {
  customInterval,
  type readOverviewFilters,
} from "./reporting.selection";
import { presetRequest, viewRequest, workflowMetrics } from "./reporting.views";

type Catalog = components["schemas"]["ReportingCatalog"];
type Filters = ReturnType<typeof readOverviewFilters>;
export const OVERVIEW_DEFAULT_PERIOD = {
  sales: "this_quarter",
  prospecting: "this_week",
  team: "this_week",
} as const satisfies Readonly<
  Record<OverviewView, ReportingSelection["period"]>
>;

export function prospectingScope(context: AnalyticsContext) {
  return context.allowed_scopes.find((scope) => scope.kind === "owner");
}

export function teamPeriod(value: string | null | undefined) {
  return (
    TEAM_PERIODS.find((period) => period === value) ??
    OVERVIEW_DEFAULT_PERIOD.team
  );
}

export function salesQuestion(
  catalog: Catalog,
  scope: AnalyticsScope,
  filters: Filters,
  zone: string,
) {
  const { metrics, blocks } = viewRequest("sales", catalog, filters.period);
  const selection: ReportingSelection = {
    scope: { kind: scope.kind, id: scope.id ?? undefined, label: scope.label },
    pipeline_id: filters.pipelineId || undefined,
    period: filters.period,
    interval: customInterval(filters.period, filters.start, filters.end, zone),
    target_basis: filters.targetBasis,
    close_window: filters.closeWindow,
    ...filters.dimensions,
    metrics,
    blocks: [],
  };
  return { selection, blocks };
}

export function prospectingQuestion(
  catalog: Catalog,
  self: AnalyticsScope | undefined,
  filters: Filters,
  zone: string,
  template: string | undefined,
) {
  const { metrics: offered, blocks: sdrBlocks } = viewRequest(
    "sdr",
    catalog,
    filters.period,
  );
  const selection: ReportingSelection = {
    scope: { kind: "owner", id: self?.id, label: self?.label },
    period: filters.period,
    interval: customInterval(filters.period, filters.start, filters.end, zone),
    target_basis: "month",
    close_window: "all_open",
    metrics: workflowMetrics(offered, template),
    blocks: [],
  };
  return { selection, sdrBlocks };
}

export function teamQuestion(
  catalog: Catalog,
  scope: AnalyticsScope,
  period: ReturnType<typeof teamPeriod>,
  template: string | undefined,
) {
  const { metrics: offered, blocks } = presetRequest("team", catalog);
  const selection: ReportingSelection = {
    scope: { kind: scope.kind, id: scope.id ?? undefined, label: scope.label },
    period,
    target_basis: "month",
    close_window: "all_open",
    metrics: workflowMetrics(offered, template),
    blocks: [],
  };
  return { selection, blocks };
}

export function prospectingTemplate(template: string): string {
  return template === "sdr" ? "sdr" : "sales";
}
