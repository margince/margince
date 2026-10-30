// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import {
  EmptyState,
  PendingBody,
  SegmentedControl,
} from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { currentParams, replaceParams, useUrlParams } from "../../lib/urlstate";
import type { AnalyticsScope } from "./analytics.context";
import {
  OVERVIEW_DIAL,
  type OverviewView,
  openingView,
  overviewViews,
  recommendedView,
  rememberedView,
  rememberView,
} from "./analytics.overview.choice";
import { MyProspecting } from "./analytics.overview.prospecting";
import { SalesReview } from "./analytics.overview.sales";
import { TeamProspecting } from "./analytics.overview.team";
import {
  useReportingCatalog,
  useReportingFramework,
} from "./reporting.queries";

type AnalyticsContext = components["schemas"]["AnalyticsContext"];

// The dials that belong to one view's period and population. A switch of view
// drops them, so each view opens on its own period: the quarter for sales,
// this week for prospecting.
const VIEW_DIALS = ["period", "from", "through", "pipeline", "close"];

/**
 * Analytics' opening page: Sales review or My prospecting, prepared from the
 * server's presets. The address names the view, then the reader's last choice,
 * then the one their work suggests.
 */
export function AnalyticsOverview({
  context,
  scope,
  scopeControl,
  emptyNotice,
}: Readonly<{
  context: AnalyticsContext;
  scope: AnalyticsScope;
  scopeControl: ReactNode;
  emptyNotice: (holdsDeals: boolean | undefined) => ReactNode;
}>) {
  const t = useT();
  const [params] = useUrlParams();
  const catalog = useReportingCatalog();
  const framework = useReportingFramework(
    useCan("reporting_framework", "read"),
  );
  return (
    <QueryGate query={catalog} pendingLabel={t("analytics.sectionOverview")}>
      {(catalog) => {
        const offered = overviewViews(
          catalog,
          context.default_scope.kind !== "owner",
        );
        const chosen = openingView(
          offered,
          params.get(OVERVIEW_DIAL),
          rememberedView(),
          undefined,
        );
        // Only a recommendation waits on the framework, so the wrong view
        // never asks for its figures first.
        if (!chosen && offered.length > 1 && framework.isLoading)
          return <PendingBody label={t("analytics.sectionOverview")} />;
        const view =
          chosen ??
          recommendedView(offered, {
            sdrFramework: framework.data?.definition.template === "sdr",
            defaultScope: context.default_scope.kind,
            submitsForecast: context.capabilities.submit_manager_forecast,
          });
        if (!view) return <EmptyState>{t("reporting.noViews")}</EmptyState>;
        const switcher =
          offered.length > 1 ? (
            <SegmentedControl
              label={t("analytics.overview.views")}
              options={offered}
              value={view}
              onChange={chooseView}
              labels={{
                sales: t("analytics.overview.sales"),
                prospecting: t("analytics.overview.prospecting"),
                team: t("analytics.overview.team"),
              }}
            />
          ) : null;
        if (view === "team")
          return (
            <TeamProspecting
              key={view}
              catalog={catalog}
              scope={scope}
              workflow={framework}
              switcher={switcher}
              scopeControl={scopeControl}
            />
          );
        return view === "sales" ? (
          <SalesReview
            key={view}
            catalog={catalog}
            scope={scope}
            switcher={switcher}
            scopeControl={scopeControl}
            emptyNotice={emptyNotice}
          />
        ) : (
          <MyProspecting
            key={view}
            catalog={catalog}
            context={context}
            workflow={framework}
            switcher={switcher}
          />
        );
      }}
    </QueryGate>
  );
}

// An explicit choice is remembered and written to the address, so Back from a
// record and a copied link both open the same view.
function chooseView(view: OverviewView): void {
  rememberView(view);
  const next = new Map(currentParams());
  for (const dial of VIEW_DIALS) next.delete(dial);
  next.set(OVERVIEW_DIAL, view);
  replaceParams(next);
}
