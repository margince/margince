import type { QueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { grants } from "../../lib/accessgrants";
import { sectionFromAddress } from "../../lib/analytics.address";
import { renderableCompanyZone } from "../../lib/companyzone";
import { pipelinesOptions } from "../../lib/pipelines.queries";
import { installationSettingsOptions } from "../../lib/uploadlimit";
import type { UrlParams } from "../../lib/urlstate";
import {
  analyticsContextOptions,
  resolveAnalyticsSelection,
} from "./analytics.context";
import { type OverviewView, readsOverview } from "./analytics.overview.choice";
import { withContext } from "./analytics.overview.context";
import { overviewOpening } from "./analytics.overview.opening";
import {
  OVERVIEW_DEFAULT_PERIOD,
  prospectingQuestion,
  prospectingScope,
  prospectingTemplate,
  salesQuestion,
  teamPeriod,
  teamQuestion,
} from "./analytics.overview.questions";
import {
  reportingCatalogOptions,
  reportingEvaluationOptions,
  reportingFrameworkOptions,
} from "./reporting.queries";
import {
  pipelineIsGone,
  readOverviewFilters,
  validDateRange,
} from "./reporting.selection";

type Me = components["schemas"]["MeResponse"];

export async function prefetchOverview(
  client: QueryClient,
  me: Me,
  segment: string | undefined,
  params: UrlParams,
): Promise<void> {
  if (
    !readsOverview({
      reports: grants(me, "report_definition", "read"),
      framework: grants(me, "reporting_framework", "read"),
    })
  )
    return;
  const context = client.getQueryData(analyticsContextOptions.queryKey);
  const catalog = client.getQueryData(reportingCatalogOptions.queryKey);
  const framework = client.getQueryData(reportingFrameworkOptions.queryKey);
  const settings = client.getQueryData(installationSettingsOptions.queryKey);
  if (!context || !catalog || !settings) return;
  if (
    sectionFromAddress(segment, "overview", params.get("view")) !== "overview"
  )
    return;
  const { view, restore } = overviewOpening({
    catalog,
    context,
    params,
    account: me.user.id,
    framework: {
      data: framework,
      isLoading:
        client.getQueryState(reportingFrameworkOptions.queryKey)?.status ===
        "pending",
    },
  });
  if (!view) return;
  const dials = restore ? withContext(params, view, restore) : params;
  const { selection, refusedScope } = resolveAnalyticsSelection(
    context,
    dials.get("scope"),
  );
  if (!selection || refusedScope) return;
  const pipelines = client.getQueryData(pipelinesOptions.queryKey);
  if (
    view === "sales" &&
    (!pipelines || pipelineIsGone(dials.get("pipeline"), pipelines))
  )
    return;
  const filters = readOverviewFilters(
    dials,
    view === "sales" ? (pipelines ?? []) : [],
    OVERVIEW_DEFAULT_PERIOD[view],
  );
  if (!validDateRange(filters.period, filters.start, filters.end)) return;
  const zone = renderableCompanyZone(settings.timezone);
  const question = questionForView(
    view,
    catalog,
    context,
    selection.scope,
    filters,
    zone,
    framework?.definition.template,
    dials.get("period"),
  );
  if (!question?.metrics.length) return;
  await client.prefetchQuery(reportingEvaluationOptions(question));
}

function questionForView(
  view: OverviewView,
  catalog: components["schemas"]["ReportingCatalog"],
  context: components["schemas"]["AnalyticsContext"],
  scope: components["schemas"]["AnalyticsScope"],
  filters: ReturnType<typeof readOverviewFilters>,
  zone: string,
  template: string | undefined,
  period: string | undefined,
) {
  if (view === "sales")
    return salesQuestion(catalog, scope, filters, zone).selection;
  if (!template) return undefined;
  if (view === "team")
    return scope.kind === "owner"
      ? undefined
      : teamQuestion(catalog, scope, teamPeriod(period), template).selection;
  const self = prospectingScope(context);
  return self?.id
    ? prospectingQuestion(
        catalog,
        self,
        filters,
        zone,
        prospectingTemplate(template),
      ).selection
    : undefined;
}
