import type { components } from "../../api/schema";
import type { UrlParams } from "../../lib/urlstate";
import {
  type AnalyticsContext,
  type AnalyticsScope,
  scopeKey,
} from "./analytics.context";
import {
  OVERVIEW_DIAL,
  type OverviewView,
  openingView,
  overviewViews,
  recommendedView,
} from "./analytics.overview.choice";
import {
  namesContext,
  type OverviewDials,
  permittedDials,
  rememberedContext,
  rememberedView,
} from "./analytics.overview.context";

type Catalog = components["schemas"]["ReportingCatalog"];
type Framework = components["schemas"]["ReportingFramework"];

export function overviewOpening({
  catalog,
  context,
  params,
  framework,
  account,
}: Readonly<{
  catalog: Catalog;
  context: AnalyticsContext;
  params: UrlParams;
  framework: Readonly<{ data?: Framework; isLoading: boolean }>;
  account: string | null;
}>) {
  const offered = overviewViews(
    catalog,
    context.default_scope.kind !== "owner",
  );
  const chosen = openingView(
    offered,
    params.get(OVERVIEW_DIAL),
    account ? rememberedView(account) : null,
    undefined,
  );
  const waitsForWork = !chosen && offered.length > 1 && framework.isLoading;
  const view = waitsForWork
    ? undefined
    : (chosen ??
      recommendedView(offered, {
        sdrFramework: framework.data?.definition.template === "sdr",
        defaultScope: context.default_scope.kind,
        submitsForecast: context.capabilities.submit_manager_forecast,
      }));
  const restore = view
    ? restoredDials(params, view, account, context.allowed_scopes)
    : undefined;
  return { offered, view, restore, waitsForWork };
}

/**
 * The question `view` opens with when the address names none. It is what
 * this tab or this account kept, minus what the reader may no longer ask.
 * Undefined when the address already states one or nothing is kept.
 */
function restoredDials(
  params: UrlParams,
  view: OverviewView,
  account: string | null,
  allowed: readonly AnalyticsScope[],
): OverviewDials | undefined {
  if (!account || namesContext(params, view)) return undefined;
  const kept = permittedDials(
    rememberedContext(account, view) ?? {},
    view,
    allowed.map(scopeKey),
  );
  return Object.keys(kept).length > 0 ? kept : undefined;
}
