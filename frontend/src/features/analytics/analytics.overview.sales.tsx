// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { QueryGate } from "../../design-system/querystates";
import { useLocale, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { useCompanyZone } from "../../lib/companyzone";
import { defaultPipelineOf, usePipelines } from "../../lib/pipelines.queries";
import { problemCodeOf } from "../../lib/problem";
import { replaceDial, type UrlParams, useUrlParams } from "../../lib/urlstate";
import { AssessmentPanel } from "./analytics.assessment";
import { AnalyticsBoard, AnalyticsSheet } from "./analytics.board";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import { DecisionsPanel } from "./analytics.decisions";
import { ReportingEvidenceDrawer } from "./analytics.drawers";
import { useSalesCharts } from "./analytics.overview.board";
import {
  type OverviewDials,
  outboundDials,
  scopeDials,
} from "./analytics.overview.context";
import { InitiativePanel } from "./analytics.overview.initiative";
import {
  InterventionPanel,
  useInterventions,
} from "./analytics.overview.interventions";
import { useOverviewNavigation } from "./analytics.overview.navigation";
import {
  OVERVIEW_DEFAULT_PERIOD,
  salesQuestion,
} from "./analytics.overview.questions";
import { metricOf } from "./analytics.overview.readings";
import { SalesFigures } from "./analytics.overview.salesfigures";
import { SalesHeader } from "./analytics.overview.salesheader";
import { SinceLastReview } from "./analytics.overview.since";
import { periodCaption } from "./analytics.period";
import { applyDimensions } from "./reporting.dimensions";
import { useSelectionEvidence } from "./reporting.evidenceselection";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
  ReportingSelection,
} from "./reporting.model";
import { useReportingEvaluation } from "./reporting.queries";
import {
  pipelineIsGone,
  useOverviewFilters,
  validDateRange,
} from "./reporting.selection";

type Catalog = components["schemas"]["ReportingCatalog"];
type Pipeline = components["schemas"]["Pipeline"];

/**
 * Sales review: where the selected population stands this fiscal quarter, in
 * four readings, a short explanation and the checks that need someone. The
 * figures and the comparison are one evaluation of the server's sales preset.
 */
export function SalesReview(
  props: Readonly<{
    catalog: Catalog;
    scope: AnalyticsScope;
    switcher: ReactNode;
    scopeControl: ReactNode;
    emptyNotice: (holdsDeals: boolean | undefined) => ReactNode;
  }>,
) {
  const t = useT();
  const pipelines = usePipelines();
  // The default pipeline is part of the question, so the page asks once it is
  // known rather than asking twice.
  return (
    <QueryGate query={pipelines} pendingLabel={t("analytics.overview.sales")}>
      {(pipelines) => <SalesReviewBody {...props} pipelines={pipelines} />}
    </QueryGate>
  );
}

function SalesReviewBody({
  catalog,
  scope,
  switcher,
  scopeControl,
  emptyNotice,
  pipelines,
}: Readonly<{
  catalog: Catalog;
  scope: AnalyticsScope;
  switcher: ReactNode;
  scopeControl: ReactNode;
  emptyNotice: (holdsDeals: boolean | undefined) => ReactNode;
  pipelines: readonly Pipeline[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useCompanyZone();
  const [params] = useUrlParams();
  const filters = useOverviewFilters(pipelines, OVERVIEW_DEFAULT_PERIOD.sales);
  const { selection, blocks } = salesQuestion(catalog, scope, filters, zone);
  const { metrics } = selection;
  const validDates = validDateRange(filters.period, filters.start, filters.end);
  // A pipeline the address names and this reader cannot open is said, never
  // swapped for the default one.
  const pipelineGone = pipelineIsGone(params.get("pipeline"), pipelines);
  const asks = validDates && metrics.length > 0 && !pipelineGone;
  // The figures and the charts are two reads of one question, so a slow or
  // failed chart never holds back the figures or the actions.
  useOverviewNavigation("sales", selection, {
    start: filters.start,
    end: filters.end,
  });
  const query = useReportingEvaluation(selection, asks);
  const charts = useSalesCharts(selection, blocks, catalog, asks);
  const { evidence, evidenceFrom, openEvidence, closeEvidence } =
    useSelectionEvidence(JSON.stringify(selection), query.data);
  const doors = outboundDials(
    "sales",
    answeredDials(selection, filters.start, filters.end),
  );
  const apply = (next: ReportingSelection) => {
    changeFilters(filters, selection, {
      ...next,
      close_window: filters.closeWindow,
    });
    if (scopeKey(next.scope) !== scopeKey(scope))
      replaceDial("scope", scopeKey(next.scope));
  };
  const checks = useInterventions(scope, filters.dimensions);
  const invalid = problemCodeOf(query.error) === "reporting_interval_invalid";
  // The evaluation the page draws below its readings; a cached answer to a
  // question the page no longer asks is never drawn.
  const answered = pipelineGone || query.isError ? undefined : query.data;
  const forecastHref = analyticsSectionHref("forecast", doors);
  const targetSetup = (
    <TargetSetup
      href={analyticsSectionHref("targets", scopeDials(scopeKey(scope)))}
    />
  );
  return (
    <>
      <SalesHeader
        selection={selection}
        pipelines={pipelines}
        switcher={switcher}
        scopeControl={scopeControl}
        dates={{
          from: filters.start,
          through: filters.end,
          onChange: (from, through) => {
            filters.setStart(from);
            filters.setEnd(through);
          },
        }}
        caption={
          validDates
            ? periodCaption(query.data, locale, t)
            : t("reporting.chooseDates")
        }
        evaluation={query.data}
        charts={charts}
        saved={query.data ? { ...query.data.selection, blocks } : undefined}
        scope={scopeKey(scope)}
        onChange={(next) => changeFilters(filters, selection, next)}
      />
      <AnalyticsBoard
        railLabel={t("analytics.board.rail")}
        main={
          pipelineGone ? (
            <PipelineGone defaultPipeline={defaultPipelineOf(pipelines)} />
          ) : (
            <>
              {answered && (
                <SalesNotes
                  notice={emptyNotice(answered.context.scope_holds_deals)}
                  evaluation={answered}
                  selection={selection}
                  period={filters.period}
                  catalog={catalog}
                  onApply={apply}
                  onEvidence={openEvidence}
                />
              )}
              <AnalyticsSheet>
                <SalesFigures
                  query={query}
                  invalid={invalid}
                  forecastHref={forecastHref}
                  charts={charts}
                  onApply={apply}
                  onEvidence={openEvidence}
                  targetSetup={targetSetup}
                />
                {/* Its own read, so it stands whatever the sales read does. */}
                <SinceLastReview
                  catalog={catalog}
                  scope={selection.scope}
                  pipelineId={selection.pipeline_id}
                  after={
                    checks.state === "not_permitted" ? "ready" : checks.state
                  }
                />
              </AnalyticsSheet>
            </>
          )
        }
        rail={
          <>
            {answered && (
              <AssessmentPanel
                selection={selection}
                evaluation={answered}
                onEvidence={openEvidence}
              />
            )}
            <InterventionPanel
              interventions={checks}
              dials={findingDials(doors)}
              emptyLabel=""
            />
            <DecisionsPanel />
          </>
        }
      />
      {/* The records behind every figure and chart on the page, under the
          read each came from. */}
      {answered && evidence && (
        <ReportingEvidenceDrawer
          key={JSON.stringify(evidence)}
          evaluation={evidenceFrom ?? answered}
          reference={evidence}
          onClose={closeEvidence}
        />
      )}
    </>
  );
}

// A filter change moves only the dials it changed, so the rest of the
// address stays as the reader left it.
function changeFilters(
  filters: ReturnType<typeof useOverviewFilters>,
  selection: ReportingSelection,
  next: ReportingSelection,
) {
  if (next.period !== filters.period) filters.setPeriod(next.period);
  if (next.pipeline_id !== selection.pipeline_id)
    filters.setPipelineId(next.pipeline_id ?? "");
  if (next.close_window !== filters.closeWindow)
    filters.setCloseWindow(next.close_window);
  applyDimensions(filters, next);
}

// The question Sales review answered, defaults spelled out: the default
// pipeline by its id, and every pipeline as `all`.
function answeredDials(
  selection: ReportingSelection,
  from: string,
  through: string,
): OverviewDials {
  const custom = selection.period === "custom";
  return {
    scope: scopeKey(selection.scope),
    pipeline: selection.pipeline_id ?? "all",
    period: selection.period,
    from: custom ? from : undefined,
    through: custom ? through : undefined,
    close: selection.close_window,
  };
}

// The findings are read for the scope over every pipeline. A door to one
// carries the scope alone, so the page it opens reads what the list read.
function findingDials(doors: UrlParams): UrlParams {
  const scope = doors.get("scope");
  return new Map([
    ["overview", "sales"],
    ...(scope ? [["scope", scope] as const] : []),
  ]);
}

// What stands above the sheet once the figures answered: why the scope is
// empty, the initiative the cut follows and the note on the period.
function SalesNotes({
  notice,
  evaluation,
  selection,
  period,
  catalog,
  onApply,
  onEvidence,
}: Readonly<{
  notice: ReactNode;
  evaluation: ReportingEvaluation;
  selection: ReportingSelection;
  period: ReportingSelection["period"];
  catalog: Catalog;
  onApply: (selection: ReportingSelection) => void;
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
}>) {
  return (
    <>
      {notice}
      {selection.initiative_id && (
        <InitiativePanel selection={selection} onEvidence={onEvidence} />
      )}
      <QuarterNotes
        evaluation={evaluation}
        period={period}
        offersLanding={catalog.metrics.some(
          (metric) => metric.id === "forecast_landing",
        )}
        onApply={onApply}
      />
    </>
  );
}

// A period other than this quarter has no expected finish to compare, and
// the one step that gives it one. The comparison's own states are explained
// in the hero, beside the attainment they affect.
function QuarterNotes({
  evaluation,
  period,
  offersLanding,
  onApply,
}: Readonly<{
  evaluation: ReportingEvaluation;
  period: ReportingSelection["period"];
  offersLanding: boolean;
  onApply: (selection: ReportingSelection) => void;
}>) {
  const t = useT();
  if (metricOf(evaluation, "forecast_landing")?.target_comparison) return null;
  if (!offersLanding || period === "this_quarter") return null;
  return (
    <p className="t-caption">
      <Button
        variant="link"
        onClick={() =>
          onApply({ ...evaluation.selection, period: "this_quarter" })
        }
      >
        {t("reporting.compare.apply.period")}
      </Button>
    </p>
  );
}

// Who can give the quarter a target. A reader who may not set one is told who
// can, rather than shown a button the server would refuse. `href` opens the
// targets for the scope the comparison was about.
// One line in the lead, beside the figure it would compare.
function TargetSetup({ href }: Readonly<{ href: string }>) {
  const t = useT();
  const canSet = useCanWrite("sales_target", "create");
  return (
    <p className="reporting-compare-note">
      {t("reporting.compare.no_target")}{" "}
      {canSet ? (
        <AppLink href={href}>{t("analytics.overview.setTarget")}</AppLink>
      ) : (
        t("analytics.overview.targetWho")
      )}
    </p>
  );
}

// The address names a pipeline this reader cannot open: archived, removed or
// never theirs. Nothing is read until they choose one the page can answer,
// and the choice is written out so it stays theirs.
function PipelineGone({
  defaultPipeline,
}: Readonly<{ defaultPipeline: Pipeline | undefined }>) {
  const t = useT();
  return (
    <Callout
      tone="warning"
      kind="standing"
      title={t("analytics.overview.pipelineGone")}
      actions={
        <>
          {defaultPipeline && (
            <Button onClick={() => replaceDial("pipeline", defaultPipeline.id)}>
              {t("analytics.overview.pipelineDefault")}
            </Button>
          )}
          <Button
            variant="ghost"
            onClick={() => replaceDial("pipeline", "all")}
          >
            {t("analytics.overview.pipelineAll")}
          </Button>
        </>
      }
    >
      <p>{t("analytics.overview.pipelineGoneBody")}</p>
    </Callout>
  );
}
