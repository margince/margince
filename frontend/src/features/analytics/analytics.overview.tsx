// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import { type ReactNode, useLayoutEffect, useRef } from "react";
import type { components } from "../../api/schema";
import {
  EmptyState,
  PendingBody,
  SegmentedControl,
} from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { useMe } from "../../lib/session";
import {
  currentParams,
  replaceParams,
  type UrlParams,
  useUrlParams,
} from "../../lib/urlstate";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import {
  OVERVIEW_DIAL,
  type OverviewView,
  openingView,
  overviewViews,
  recommendedView,
} from "./analytics.overview.choice";
import {
  chooseContext,
  dialsOf,
  namesContext,
  noteContext,
  type OverviewDials,
  permittedDials,
  rememberedContext,
  rememberedView,
  rememberView,
  withContext,
} from "./analytics.overview.context";
import { MyProspecting } from "./analytics.overview.prospecting";
import { SalesReview } from "./analytics.overview.sales";
import { TeamProspecting } from "./analytics.overview.team";
import { useReturnFocus } from "./analytics.returnfocus";
import {
  useReportingCatalog,
  useReportingFramework,
} from "./reporting.queries";

type AnalyticsContext = components["schemas"]["AnalyticsContext"];
type Catalog = components["schemas"]["ReportingCatalog"];
type Framework = components["schemas"]["ReportingFramework"];

type OverviewProps = Readonly<{
  context: AnalyticsContext;
  scope: AnalyticsScope;
  scopeControl: ReactNode;
  emptyNotice: (holdsDeals: boolean | undefined) => ReactNode;
}>;

/**
 * Analytics' opening page: Sales review, My prospecting or Team prospecting,
 * prepared from the server's presets. The address names the view, then the
 * reader's last choice, then the one their work suggests. Each view keeps
 * its own question the same way: the address, then what was remembered.
 */
export function AnalyticsOverview(props: OverviewProps) {
  const t = useT();
  const catalog = useReportingCatalog();
  const me = useMe();
  const framework = useReportingFramework(
    useCan("reporting_framework", "read"),
  );
  const surface = useRef<HTMLDivElement>(null);
  useReturnFocus(surface);
  return (
    <div ref={surface} className="analytics-overview">
      {/* What was remembered belongs to an account, so the page waits for
          the session rather than asking twice. */}
      {me.isPending ? (
        <PendingBody label={t("analytics.sectionOverview")} />
      ) : (
        <QueryGate
          query={catalog}
          pendingLabel={t("analytics.sectionOverview")}
        >
          {(catalog) => (
            <OverviewBoard
              {...props}
              catalog={catalog}
              framework={framework}
              account={me.data?.user.id ?? null}
            />
          )}
        </QueryGate>
      )}
    </div>
  );
}

function OverviewBoard({
  context,
  scope,
  scopeControl,
  emptyNotice,
  catalog,
  framework,
  account,
}: OverviewProps &
  Readonly<{
    catalog: Catalog;
    framework: UseQueryResult<Framework>;
    account: string | null;
  }>) {
  const t = useT();
  const [params] = useUrlParams();
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
  useOverviewMemory(account, view, params, restore);
  // Only a recommendation waits on the framework, and a remembered question
  // is put in the address first, so the wrong figures are never asked for.
  if (waitsForWork || restore)
    return <PendingBody label={t("analytics.sectionOverview")} />;
  if (!view) return <EmptyState>{t("reporting.noViews")}</EmptyState>;
  const switcher =
    offered.length > 1 ? (
      <SegmentedControl
        label={t("analytics.overview.views")}
        options={offered}
        value={view}
        onChange={(next) => chooseView(account, next)}
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
        defaultScope={context.default_scope}
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
      defaultScope={context.default_scope}
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
}

/**
 * The question `view` opens with when the address names none: what this tab
 * or this account kept for it, minus what the reader may no longer ask.
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

/**
 * Puts a remembered question in the address, and keeps each view's question
 * as it changes. Every question shown is noted for this tab. Only a dial the
 * reader turns becomes their own choice, so opening a shared link changes
 * nothing they chose.
 */
function useOverviewMemory(
  account: string | null,
  view: OverviewView | undefined,
  params: UrlParams,
  restore: OverviewDials | undefined,
): void {
  const shown = useRef<string | null>(null);
  const dials = view ? dialsOf(params, view) : {};
  const signature = `${view}:${JSON.stringify(restore ?? dials)}`;
  // biome-ignore lint/correctness/useExhaustiveDependencies: the signature spells view, dials and restore
  useLayoutEffect(() => {
    if (!account || !view) return;
    const before = shown.current;
    shown.current = signature;
    if (restore) {
      replaceParams(withContext(currentParams(), view, restore));
      return;
    }
    const turned = before?.startsWith(`${view}:`) && before !== signature;
    if (turned) chooseContext(account, view, dials);
    else noteContext(account, view, dials);
  }, [account, signature]);
}

// An explicit choice of view is remembered and written to the address, so
// Back from a record and a copied link both open the same view. The view
// opens on its own question, never on the one just left.
function chooseView(account: string | null, view: OverviewView): void {
  if (account) rememberView(account, view);
  replaceParams(withContext(currentParams(), view, {}));
}
