import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { FilterBar } from "../../design-system/filterbar";
import { Popover } from "../../design-system/popover";
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
import type { Stage } from "./analytics.cells";
import {
  type AnalyticsSelection,
  scopeKey,
  useAnalyticsContext,
  useAnalyticsSelection,
} from "./analytics.context";
import { DataCoverageView } from "./analytics.coverageview";
import { FirstRunNotice } from "./analytics.firstrun";
import { ForecastView, SharedForecastView } from "./analytics.forecast";
import { MyOutcomesView } from "./analytics.outcomes";
import { QuestionsView } from "./analytics.questions";
import { AnalyticsScopePicker } from "./analytics.scope";
import { SETUP_LABEL, useAnalyticsSections } from "./analytics.tabs";
import { stagesOfEveryPipeline } from "./pipelinestages";
import { ReportingDefinitions } from "./reporting.definitions";
import { ReportingLibrary } from "./reporting.library";
import { ReportingOverview } from "./reporting.overview";
import { ReportingTargets } from "./reporting.targets";
import { VIEW_SHAPE, viewOfSection } from "./reporting.views";
import "./analytics.css";

// Analytics: a picker over three reports — deals-by-stage (unweighted beside
// weighted), forecast (category readings plus the server-derived "slipped"
// bucket), and open deals per company. "Explain this number" opens the executed
// plan and the exact rows the headline reconciles to. Both weighted figures come
// straight off the report's own weighted_amount_minor measure, rounded PER DEAL
// and then summed, so neither screen re-derives one from the raw total. All
// three bodies render into ONE panel whose action band carries the explain
// toggle: the segment changes what the panel holds, not what the page is.
import { ReportCard, SECTION_REPORTS, useReportRun } from "./analytics.reports";

export { buildStageAggregates, ForecastTile } from "./analytics.pipeline";

export function AnalyticsScreen({
  reviewActions,
}: Readonly<{ reviewActions?: ReviewActions }> = {}) {
  const inheritedReviewActions = useReviewActions();
  const t = useT();
  const { locale } = useLocale();
  // The server decides which population this reader measures and which ones
  // they may choose. Read once here and handed down, so every card on the page
  // is answering about the same set.
  const context = useAnalyticsContext();
  const { selection, selectScope } = useAnalyticsSelection(context.data);

  const pipelinesQuery = useQuery({
    queryKey: ["pipelines", "with-archived"],
    // Archived too: a deal keeps its stage when its pipeline is archived.
    queryFn: async () =>
      unwrap(
        await api.GET("/pipelines", {
          params: { query: { include_archived: true } },
        }),
      ).data,
  });

  const { route, section, tabs, setup, library, landing } =
    useAnalyticsSections(context.data?.default_scope.kind);
  // Which population the figures cover, drawn first in the section's own
  // filter bar so every filter on the page sits in one row.
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
    return <SharedForecastView token={route.id2} />;

  return (
    <div className="wrap">
      <RecordTabs
        options={tabs}
        // Reports and Setup pages mark no tab: the button beside the row names
        // where the reader is.
        value={section}
        onChange={openAnalyticsSection}
        labels={{
          reports: t("analytics.sectionReports"),
          targets: t("reporting.targets"),
          definitions: t("reporting.definitions"),
          forecast: t("analytics.sectionForecast"),
          pipeline: t("analytics.sectionPipeline"),
          performance: t("analytics.sectionPerformance"),
          sdr: t(VIEW_SHAPE.sdr.label),
          leads: t(VIEW_SHAPE.leads.label),
          partners: t(VIEW_SHAPE.partners.label),
          outcomes: t("analytics.sectionOutcomes"),
          coverage: t("analytics.sectionCoverage"),
          delivery: t("analytics.sectionDelivery"),
          questions: t("analytics.sectionQuestions"),
        }}
        label={t("analytics.sections")}
        trailing={
          library || setup.length > 0 ? (
            <>
              {library && (
                <Button
                  variant="ghost"
                  onClick={() => openAnalyticsSection(library)}
                >
                  {t(
                    library === "reports"
                      ? "analytics.sectionReports"
                      : "analytics.sectionQuestions",
                  )}
                </Button>
              )}
              {setup.length > 0 && (
                <Popover label={t("analytics.setup")}>
                  {setup.map((candidate) => (
                    <Button
                      key={candidate}
                      variant="link"
                      onClick={() => openAnalyticsSection(candidate)}
                    >
                      {t(SETUP_LABEL[candidate])}
                    </Button>
                  ))}
                </Popover>
              )}
            </>
          ) : undefined
        }
      />
      <div className="analytics-body">
        <ReviewActionsContext value={reviewActions ?? inheritedReviewActions}>
          {landing ? (
            <QueryStates
              query={landing}
              pendingLabel={t("analytics.sectionPerformance")}
            >
              {null}
            </QueryStates>
          ) : (
            <SectionBody
              section={section}
              locale={locale}
              context={context.data}
              contextQuery={context}
              selection={selection}
              onSelectScope={selectScope}
              scopeControl={scopeControl}
              stages={stagesOfEveryPipeline(pipelinesQuery.data ?? [], (name) =>
                t("analytics.retiredPipeline", { name }),
              )}
            />
          )}
        </ReviewActionsContext>
      </div>
    </div>
  );
}

// One section's body, chosen in its own component so the screen's own render
// stays a header plus a choice rather than a ladder of ternaries.
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
      // Nothing renders before the frame arrives, so no zone is guessed.
      return context ? (
        <DataCoverageView locale={locale} timezone={context.timezone} />
      ) : null;
    case "outcomes":
      return context ? (
        <MyOutcomesView defaultScope={context.default_scope} locale={locale} />
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
          {SECTION_REPORTS[section].map((report) => (
            <ReportCard
              key={report}
              report={report}
              stages={stages}
              locale={locale}
            />
          ))}
        </>
      );
  }
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
        <FilterBar label={t("analytics.filters")}>{scopeControl}</FilterBar>
      )}
      {emptyNotice(first.data?.scope_holds_deals)}
      {SECTION_REPORTS.pipeline.map((report) => (
        <ReportCard
          // Keyed by scope too, so an open explanation never outlives the
          // records it explains.
          key={scope ? `${report}:${scopeKey(scope)}` : report}
          report={report}
          stages={stages}
          locale={locale}
          scope={scope}
        />
      ))}
    </>
  );
}
