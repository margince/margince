import { useQuery } from "@tanstack/react-query";
import { type ReactNode, Suspense } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, PendingBody } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { FilterBar } from "../../design-system/filterbar";
import { QueryStates } from "../../design-system/querystates";
import { RecordTabs } from "../../design-system/recordtabs";
import { type Locale, useLocale, useT } from "../../i18n";
import {
  openAnalyticsSection,
  type Section,
} from "../../lib/analytics.address";
import { problemStatus, unwrap } from "../../lib/problem";
import {
  type ReviewActions,
  ReviewActionsContext,
  useReviewActions,
} from "../../lib/reviewactions";
import { navigate } from "../../lib/router";
import { replaceDial } from "../../lib/urlstate";
import type { Stage } from "./analytics.cells";
import {
  type AnalyticsSelection,
  scopeKey,
  useAnalyticsContext,
  useAnalyticsSelection,
} from "./analytics.context";
import { FirstRunNotice } from "./analytics.firstrun";
import { AnalyticsOverview } from "./analytics.overview";
import {
  OverviewNavigation,
  useOverviewRegistration,
} from "./analytics.overview.navigation";
import { AnalyticsScopePicker } from "./analytics.scope";
import {
  ForecastView,
  FramedSection,
  QuestionsView,
  ReportBoard,
  ReportingDefinitions,
  ReportingLibrary,
  ReportingOverview,
  ReportingTargets,
  SharedForecastView,
} from "./analytics.sections";
import { useAnalyticsSections } from "./analytics.tabs";
import { TabVerbs } from "./analytics.tabverbs";
import { stagesOfEveryPipeline } from "./pipelinestages";
import { VIEW_SHAPE, viewOfSection } from "./reporting.views";
import "./analytics.css";

import { ExtensionReports } from "../../components/extensionreports";
import { SECTION_REPORTS, useReportRun } from "./analytics.reports.queries";

export function AnalyticsScreen({
  reviewActions,
}: Readonly<{ reviewActions?: ReviewActions }> = {}) {
  const inheritedReviewActions = useReviewActions();
  const { dials: overviewDials, register: registerOverview } =
    useOverviewRegistration();
  const t = useT();
  const { locale } = useLocale();
  // The server decides which population this reader measures and which ones
  // they may choose. Read once here and handed down, so every card on the page
  // is answering about the same set.
  const context = useAnalyticsContext();
  const { selection, selectScope, refusedScope } = useAnalyticsSelection(
    context.data,
  );

  const { route, section, tabs, setup, library, holding } =
    useAnalyticsSections(context.data?.default_scope.kind);
  const pipelinesQuery = useQuery({
    queryKey: ["pipelines", "with-archived"],
    // Only report cards name stages, so the other sections never wait on this.
    enabled: !holding && SECTION_REPORTS[section].length > 0,
    // Archived too: a deal keeps its stage when its pipeline is archived.
    queryFn: async () =>
      unwrap(
        await api.GET("/pipelines", {
          params: { query: { include_archived: true } },
        }),
      ).data,
  });
  const scopeControl =
    selection && context.data ? (
      <div className="analytics-scope-control">
        <AnalyticsScopePicker
          scopes={context.data.allowed_scopes}
          selected={selection.scope}
          onSelect={selectScope}
        />
      </div>
    ) : null;

  if (route.screen === "analytics" && route.id === "shared" && route.id2)
    return (
      <Suspense
        fallback={
          <div className="wrap">
            <PendingBody label={t("analytics.sections")} />
          </div>
        }
      >
        <SharedForecastView token={route.id2} />
      </Suspense>
    );

  return (
    <div className="wrap">
      <OverviewNavigation value={registerOverview}>
        <RecordTabs
          options={tabs}
          // Reports and Setup pages mark no tab: the button beside the row names
          // where the reader is.
          value={section}
          onChange={(next) => {
            if (section === "overview" && overviewDials.current)
              navigate(
                { screen: "analytics", id: next },
                overviewDials.current,
              );
            else openAnalyticsSection(next);
          }}
          labels={{
            overview: t("analytics.sectionOverview"),
            reports: t("analytics.sectionReports"),
            targets: t("reporting.targets"),
            definitions: t("reporting.definitions"),
            forecast: t("analytics.sectionForecast"),
            pipeline: t("analytics.sectionPipeline"),
            performance: t("analytics.sectionPerformance"),
            sdr: t(VIEW_SHAPE.sdr.label),
            leads: t(VIEW_SHAPE.leads.label),
            partners: t(VIEW_SHAPE.partners.label),
            conversion: t(VIEW_SHAPE.conversion.label),
            outcomes: t("analytics.sectionOutcomes"),
            coverage: t("analytics.sectionCoverage"),
            accounts: t("analytics.sectionAccounts"),
            delivery: t("analytics.sectionDelivery"),
            questions: t("analytics.sectionQuestions"),
          }}
          label={t("analytics.sections")}
          trailing={
            <TabVerbs library={library} setup={setup} section={section} />
          }
        />
        <div className="analytics-body">
          <ReviewActionsContext value={reviewActions ?? inheritedReviewActions}>
            {holding ? (
              <PendingBody label={t("analytics.sections")} />
            ) : refusedScope && context.data ? (
              <ScopeRefused
                fallback={context.data.default_scope}
                allowed={context.data.allowed_scopes}
              />
            ) : (
              <Suspense
                fallback={<PendingBody label={t("analytics.sections")} />}
              >
                <SectionBody
                  section={section}
                  locale={locale}
                  context={context.data}
                  contextQuery={context}
                  selection={selection}
                  onSelectScope={selectScope}
                  scopeControl={scopeControl}
                  stages={stagesOfEveryPipeline(
                    pipelinesQuery.data ?? [],
                    (name) => t("analytics.retiredPipeline", { name }),
                  )}
                />
              </Suspense>
            )}
          </ReviewActionsContext>
        </div>
      </OverviewNavigation>
    </div>
  );
}

// The address names a population this reader may not choose: a link from a
// colleague who sees more, or a grant withdrawn since. Nothing is read until
// the reader takes the population they do have.
function ScopeRefused({
  fallback,
  allowed,
}: Readonly<{
  fallback: components["schemas"]["AnalyticsScope"];
  allowed: readonly components["schemas"]["AnalyticsScope"][];
}>) {
  const t = useT();
  // Named when it can be. A bare address may open a remembered question,
  // and the page must open on the population this button names.
  const named = allowed.some((scope) => scopeKey(scope) === scopeKey(fallback));
  return (
    <Callout
      tone="warning"
      kind="standing"
      title={t("analytics.scopeRefused")}
      actions={
        <Button
          onClick={() =>
            replaceDial("scope", named ? scopeKey(fallback) : undefined)
          }
        >
          {t("analytics.scopeRefusedUse", { scope: fallback.label })}
        </Button>
      }
    >
      <p>{t("analytics.scopeRefusedBody")}</p>
    </Callout>
  );
}

function SectionBody({
  section,
  locale,
  context,
  contextQuery,
  selection,
  onSelectScope,
  scopeControl,
  stages,
}: Readonly<{
  section: Section;
  locale: Locale;
  context: components["schemas"]["AnalyticsContext"] | undefined;
  contextQuery: ContextQuery;
  selection: AnalyticsSelection | null;
  onSelectScope: (scope: AnalyticsSelection["scope"]) => void;
  scopeControl: ReactNode;
  stages: readonly Stage[];
}>) {
  const t = useT();
  // Each deal section hands the notice the population flag of its own answer.
  // The message and the figures beside it then come from one server evaluation.
  const emptyNotice = (holdsDeals: boolean | undefined) =>
    selection && context ? (
      <FirstRunNotice
        holdsDeals={holdsDeals}
        scope={selection.scope}
        allowedScopes={context.allowed_scopes}
        onSelectScope={onSelectScope}
      />
    ) : null;
  const view = viewOfSection(section);
  if (view) {
    return selection ? (
      <ReportingOverview
        // Keyed by view, so an open drawer never outlives the tab it was
        // opened on.
        key={view}
        view={view}
        scope={selection.scope}
        scopeControl={scopeControl}
        emptyNotice={emptyNotice}
      />
    ) : null;
  }
  switch (section) {
    case "overview":
      return (
        <OverviewSection
          context={context}
          contextQuery={contextQuery}
          selection={selection}
          scopeControl={scopeControl}
          emptyNotice={emptyNotice}
        />
      );
    case "reports":
      return (
        <ReportingLibrary
          scope={selection?.scope}
          ownLens={context?.default_scope.kind === "owner"}
        />
      );
    case "targets":
      return <ReportingTargets />;
    case "definitions":
      return <ReportingDefinitions />;
    case "questions":
      return selection && context ? (
        <>
          <FilterBar label={t("analytics.filters")}>{scopeControl}</FilterBar>
          <QuestionsView
            context={context}
            selection={selection}
            onSelectScope={onSelectScope}
          />
        </>
      ) : null;
    case "coverage":
    case "accounts":
    case "outcomes":
      // Nothing renders before the frame arrives, so no zone is guessed.
      return context ? (
        <FramedSection
          section={section}
          context={context}
          selection={selection}
          scopeControl={scopeControl}
          locale={locale}
        />
      ) : null;
    case "forecast":
      return selection && context ? (
        <ForecastView
          selection={selection}
          canSubmit={context.capabilities.submit_manager_forecast}
          scopeControl={scopeControl}
          emptyNotice={emptyNotice}
        />
      ) : null;
    case "pipeline":
      return (
        <PipelineSection
          selection={selection}
          contextQuery={contextQuery}
          scopeControl={scopeControl}
          stages={stages}
          locale={locale}
          emptyNotice={emptyNotice}
        />
      );
    default:
      return (
        <>
          {section === "delivery" && (
            <p className="sub">{t("analytics.deliveryCoversAll")}</p>
          )}
          <ReportBoard
            reports={SECTION_REPORTS[section]}
            stages={stages}
            locale={locale}
          />
        </>
      );
  }
}

function OverviewSection({
  context,
  contextQuery,
  selection,
  scopeControl,
  emptyNotice,
}: Readonly<{
  context: components["schemas"]["AnalyticsContext"] | undefined;
  contextQuery: ContextQuery;
  selection: AnalyticsSelection | null;
  scopeControl: ReactNode;
  emptyNotice: (holdsDeals: boolean | undefined) => ReactNode;
}>) {
  const t = useT();
  if (selection && context)
    return (
      <AnalyticsOverview
        context={context}
        scope={selection.scope}
        scopeControl={scopeControl}
        emptyNotice={emptyNotice}
      />
    );
  return (
    <QueryStates
      query={contextQuery}
      pendingLabel={t("analytics.sectionOverview")}
    >
      {null}
    </QueryStates>
  );
}

type ContextQuery = Readonly<{
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => unknown;
}>;

// Deals follow the picker so Pipeline and Performance count one set of
// records; Delivery does not, because a project is read company-wide. The
// picker needs forecast access, so a deal reader refused it (403) keeps the
// unscoped cards. Any other context failure is shown, never answered with
// totals for a scope the link did not ask for.
function PipelineSection({
  selection,
  contextQuery,
  scopeControl,
  stages,
  locale,
  emptyNotice,
}: Readonly<{
  selection: AnalyticsSelection | null;
  contextQuery: ContextQuery;
  scopeControl: ReactNode;
  stages: readonly Stage[];
  locale: Locale;
  emptyNotice: (holdsDeals: boolean | undefined) => ReactNode;
}>) {
  const t = useT();
  const refused = problemStatus(contextQuery.error) === 403;
  const first = useReportRun(
    SECTION_REPORTS.pipeline[0],
    selection?.scope,
    selection != null || refused,
  );
  if (!selection && !refused) {
    return (
      <QueryStates
        query={contextQuery}
        pendingLabel={t("analytics.sectionPipeline")}
      >
        {null}
      </QueryStates>
    );
  }
  const scope = selection?.scope;
  return (
    <>
      {scope && (
        <FilterBar label={t("analytics.filters")} quiet labels="inside">
          {scopeControl}
        </FilterBar>
      )}
      {emptyNotice(first.data?.scope_holds_deals)}
      <ReportBoard
        reports={SECTION_REPORTS.pipeline}
        stages={stages}
        locale={locale}
        scope={scope}
      />
      <ExtensionReports />
    </>
  );
}
