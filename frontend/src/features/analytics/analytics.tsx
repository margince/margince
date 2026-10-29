import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, EmptyState } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { Popover } from "../../design-system/popover";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { RecordTabs } from "../../design-system/recordtabs";
import { formatDateTime } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  openAnalyticsSection,
  SECTIONS,
  type Section,
  sectionFromAddress,
} from "../../lib/analytics.address";
import { useCan } from "../../lib/capability";
import { problemStatus, unwrap } from "../../lib/problem";
import {
  type ReviewActions,
  ReviewActionsContext,
  useReviewActions,
} from "../../lib/reviewactions";
import { useRoute } from "../../lib/router";
import type { Stage } from "./analytics.cells";
import {
  type AnalyticsSelection,
  scopeKey,
  useAnalyticsContext,
  useAnalyticsSelection,
} from "./analytics.context";
import { type DataCoverageRow, useDataCoverage } from "./analytics.coverage";
import { FirstRunNotice } from "./analytics.firstrun";
import { ForecastView, SharedForecastView } from "./analytics.forecast";
import { sourceName } from "./analytics.forecast.review";
import { MyOutcomesView } from "./analytics.outcomes";
import { QuestionsView } from "./analytics.questions";
import { AnalyticsScopePicker } from "./analytics.scope";
import { stagesOfEveryPipeline } from "./pipelinestages";
import { ReportingDefinitions } from "./reporting.definitions";
import { ReportingLibrary } from "./reporting.library";
import { ReportingOverview } from "./reporting.overview";
import { ReportingTargets } from "./reporting.targets";
import "./analytics.css";

// Analytics: a picker over three reports — deals-by-stage (unweighted beside
// weighted), forecast (category readings plus the server-derived "slipped"
// bucket), and open deals per company. "Explain this number" opens the executed
// plan and the exact rows the headline reconciles to. Both weighted figures come
// straight off the report's own weighted_amount_minor measure, rounded PER DEAL
// and then summed, so neither screen re-derives one from the raw total. All
// three bodies render into ONE panel whose action band carries the explain
// toggle: the segment changes what the panel holds, not what the page is.
import { ExtensionReports } from "../../components/extensionreports";
import { ReportCard, SECTION_REPORTS, useReportRun } from "./analytics.reports";

export { buildStageAggregates, ForecastTile } from "./analytics.pipeline";

// Every word a coverage row's state can carry, spelled per state so an
// unread source never borrows a read one's copy. A hand-kept mirror of the
// server's own vocabulary; an unknown state renders its raw word rather than
// a wrong sentence.
const COVERAGE_STATE_KEY: Readonly<Record<string, MessageKey>> = {
  checked: "analytics.covChecked",
  stale: "analytics.covStale",
  unavailable: "analytics.covUnavailable",
  permission_limited: "analytics.covPermissionLimited",
  not_connected: "analytics.covNotConnected",
};

// Which connectors the nightly check could read, and how far. Ops-facing: the
// server gates the read, and a seat without the grant never sees the tab.
function DataCoverageView({
  locale,
  timezone,
}: Readonly<{ locale: Locale; timezone: string }>) {
  const t = useT();
  const coverage = useDataCoverage();
  const allowed = useCan("data_coverage", "read");
  if (!allowed) {
    return <EmptyState>{t("common.permissionDenied")}</EmptyState>;
  }
  return (
    <QueryGate query={coverage} pendingLabel={t("analytics.sectionCoverage")}>
      {(run) =>
        run == null ? (
          <Panel title={t("analytics.sectionCoverage")}>
            <EmptyState>
              {t("analytics.coverageNeverRun")}
              <Button
                variant="link"
                onClick={() => openAnalyticsSection("forecast")}
              >
                {t("analytics.sectionForecast")}
              </Button>
            </EmptyState>
          </Panel>
        ) : (
          <Panel title={t("analytics.sectionCoverage")}>
            <PanelBody>
              <PanelIntro>{t("analytics.coverageSub")}</PanelIntro>
              <DataTable
                label={t("analytics.sectionCoverage")}
                columns={[
                  {
                    key: "source",
                    header: t("analytics.covSource"),
                    render: (row: DataCoverageRow) => sourceName(row.source, t),
                  },
                  {
                    key: "state",
                    header: t("analytics.covState"),
                    render: (row: DataCoverageRow) => {
                      const stateKey = COVERAGE_STATE_KEY[row.state];
                      return stateKey ? t(stateKey) : row.state;
                    },
                  },
                  {
                    key: "through",
                    header: t("analytics.covThrough"),
                    render: (row: DataCoverageRow) =>
                      row.checked_through
                        ? formatDateTime(row.checked_through, locale, timezone)
                        : "—",
                  },
                ]}
                rows={run.sources}
                rowKey={(row) => row.source}
              />
              {/* Record-level input problems live where they are answered: the
                Forecast input review. One resolution surface, not two. */}
              <Button
                variant="link"
                onClick={() => openAnalyticsSection("forecast")}
              >
                {t("review.title")}
              </Button>
              {run.sources.some((source) => source.state !== "checked") && (
                <p className="t-caption">{t("reporting.coverageAction")}</p>
              )}
            </PanelBody>
          </Panel>
        )
      }
    </QueryGate>
  );
}

export function AnalyticsScreen({
  reviewActions,
}: Readonly<{ reviewActions?: ReviewActions }> = {}) {
  const inheritedReviewActions = useReviewActions();
  const t = useT();
  const { locale } = useLocale();
  // Which SECTION is open is an ADDRESS, so a reader can link to one and Back
  // steps between the sections they looked at rather than leaving the screen.
  // Read here rather than taken as a prop, so this screen stays drivable on its
  // own: a suite that renders it directly goes on pressing the tabs.
  const route = useRoute();
  const requested = sectionFromAddress(
    route.screen === "analytics" ? route.id : undefined,
  );
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

  const canReadReports = useCan("report_definition", "read");
  const canReadTargets = useCan("sales_target", "read");
  const canReadFramework = useCan("reporting_framework", "read");
  const canReadCoverage = useCan("data_coverage", "read");
  const coverageProbe = useDataCoverage();
  const availableSections = SECTIONS.filter((candidate) => {
    if (candidate === "performance") return canReadReports && canReadFramework;
    if (candidate === "reports") return canReadReports;
    if (candidate === "targets") return canReadTargets;
    if (candidate === "definitions") return canReadFramework;
    if (candidate === "outcomes") {
      return context.data?.default_scope.kind === "owner";
    }
    if (candidate === "coverage") {
      // The read starts only with the ops grant; the tab appears
      // after the server has answered.
      return canReadCoverage && coverageProbe.isSuccess;
    }
    return true;
  });
  const section =
    ["performance", "reports", "targets", "definitions"].includes(requested) &&
    !availableSections.includes(requested)
      ? "forecast"
      : requested;
  const tabs = analyticsTabs(availableSections);
  const setup = availableSections.filter(isSetupSection);
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
        // A custom report is opened from Reports, so that tab stays marked
        // while one is open. A Setup page marks no tab: the menu names it.
        value={
          section === "questions" && tabs.includes("reports")
            ? "reports"
            : section
        }
        onChange={openAnalyticsSection}
        labels={{
          reports: t("analytics.sectionReports"),
          targets: t("reporting.targets"),
          definitions: t("reporting.definitions"),
          forecast: t("analytics.sectionForecast"),
          pipeline: t("analytics.sectionPipeline"),
          performance: t("analytics.sectionPerformance"),
          outcomes: t("analytics.sectionOutcomes"),
          coverage: t("analytics.sectionCoverage"),
          delivery: t("analytics.sectionDelivery"),
          questions: t("analytics.sectionQuestions"),
        }}
        label={t("analytics.sections")}
        trailing={
          setup.length > 0 ? (
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
          ) : undefined
        }
      />
      <div className="analytics-body">
        <ReviewActionsContext value={reviewActions ?? inheritedReviewActions}>
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
        </ReviewActionsContext>
      </div>
    </div>
  );
}

// The tab row: every analysis section this reader may open, drawn whichever
// one is open, so the row never changes shape under the reader.
const TAB_SECTIONS: readonly Section[] = [
  "performance",
  "forecast",
  "pipeline",
  "outcomes",
  "delivery",
  "reports",
];

// Administration rather than analysis, behind one menu at the row's end.
type SetupSection = Extract<Section, "targets" | "definitions" | "coverage">;

const SETUP_LABEL: Readonly<Record<SetupSection, MessageKey>> = {
  targets: "reporting.targets",
  definitions: "reporting.definitions",
  coverage: "analytics.sectionCoverage",
};

function isSetupSection(section: Section): section is SetupSection {
  return section in SETUP_LABEL;
}

// The row keeps its own order, not the address list's that `available` follows.
// Custom reports open from Reports; a reader who may not read saved reports
// still asks their own questions, so for them it is a tab of its own.
export function analyticsTabs(available: readonly Section[]): Section[] {
  const tabs = TAB_SECTIONS.filter((section) => available.includes(section));
  return tabs.includes("reports") || !available.includes("questions")
    ? tabs
    : [...tabs, "questions"];
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
  switch (section) {
    case "performance":
      return selection ? (
        <ReportingOverview
          scope={selection.scope}
          scopeControl={scopeControl}
          emptyNotice={emptyNotice}
        />
      ) : null;
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
      <ExtensionReports />
    </>
  );
}
