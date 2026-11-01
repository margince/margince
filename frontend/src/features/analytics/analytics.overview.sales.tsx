// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { QueryGate } from "../../design-system/querystates";
import { useLocale, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { usePipelines } from "../../lib/pipelines.queries";
import { problemCodeOf } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { replaceDial, type UrlParams, useUrlParams } from "../../lib/urlstate";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import {
  SalesHero,
  StockCharts,
  useSalesCharts,
} from "./analytics.overview.board";
import {
  type OverviewDials,
  outboundDials,
  scopeDials,
} from "./analytics.overview.context";
import { DecisionSummary } from "./analytics.overview.decision";
import { HeroAside } from "./analytics.overview.hero";
import {
  InterventionPanel,
  useInterventions,
} from "./analytics.overview.interventions";
import { metricOf, OverviewLinks } from "./analytics.overview.readings";
import { SalesHeader } from "./analytics.overview.salesheader";
import {
  OpenPipelineLine,
  SalesReadings,
} from "./analytics.overview.salesreadings";
import { SinceLastReview } from "./analytics.overview.since";
import { periodCaption } from "./analytics.period";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { useSelectionEvidence } from "./reporting.evidenceselection";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
  ReportingSelection,
} from "./reporting.model";
import {
  customInterval,
  useOverviewFilters,
  validDateRange,
} from "./reporting.overview";
import { useReportingEvaluation } from "./reporting.queries";
import { viewRequest } from "./reporting.views";

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
  const zone = useRecordZone();
  const [params] = useUrlParams();
  const filters = useOverviewFilters(pipelines, "this_quarter");
  const { metrics, blocks } = viewRequest("sales", catalog, filters.period);
  const selection: ReportingSelection = {
    scope: { kind: scope.kind, id: scope.id ?? undefined, label: scope.label },
    pipeline_id: filters.pipelineId || undefined,
    period: filters.period,
    interval: customInterval(filters.period, filters.start, filters.end, zone),
    target_basis: filters.targetBasis,
    close_window: filters.closeWindow,
    metrics,
    blocks: [],
  };
  const validDates = validDateRange(filters.period, filters.start, filters.end);
  // A pipeline the address names and this reader cannot open is said, never
  // swapped for the default one.
  const pipelineGone = pipelineIsGone(params.get("pipeline"), pipelines);
  const asks = validDates && metrics.length > 0 && !pipelineGone;
  // The figures and the charts are two reads of one question, so a slow or
  // failed chart never holds back the figures or the actions.
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
  const checks = useInterventions(scope);
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
      {pipelineGone ? (
        <PipelineGone
          defaultPipeline={
            pipelines.find((pipeline) => pipeline.is_default) ?? pipelines[0]
          }
        />
      ) : invalid ? (
        <ErrorLine error={query.error} />
      ) : (
        <QueryGate query={query} pendingLabel={t("analytics.overview.sales")}>
          {(evaluation) => (
            <>
              {emptyNotice(evaluation.context.scope_holds_deals)}
              <QuarterNotes
                evaluation={evaluation}
                period={filters.period}
                offersLanding={catalog.metrics.some(
                  (metric) => metric.id === "forecast_landing",
                )}
                onApply={apply}
              />
              {/* The lead and its three readings share one row, so the
                  decision and the first action still fit the first screen. */}
              <div className="analytics-lead">
                <SalesHero
                  evaluation={evaluation}
                  charts={charts}
                  onEvidence={openEvidence}
                  aside={
                    <HeroAside
                      evaluation={evaluation}
                      charts={charts?.data}
                      onApply={apply}
                      targetSetup={targetSetup}
                    />
                  }
                />
                <SalesReadings
                  evaluation={evaluation}
                  onEvidence={openEvidence}
                  forecastHref={forecastHref}
                />
              </div>
            </>
          )}
        </QueryGate>
      )}
      {/* One place for the action list, so it never moves while the figures
          load; its sources are its own, so a failed evaluation leaves it. */}
      <div className="analytics-pair">
        {answered && (
          <DecisionSummary
            evaluation={answered}
            checks={{
              state: checks.state,
              findings: checks.list?.totals?.findings,
              deals: checks.list?.totals?.deals,
            }}
            forecastHref={forecastHref}
            onEvidence={openEvidence}
          />
        )}
        <InterventionPanel
          interventions={checks}
          dials={findingDials(doors)}
          emptyLabel={t("analytics.overview.salesNothing")}
        />
      </div>
      {/* Below the decision and the actions: today's stock by stage, and the
          permitted chart of sales by owner, in name order. */}
      {answered && (
        <SalesStock
          answered={answered}
          charts={charts}
          evidence={evidence}
          evidenceFrom={evidenceFrom}
          onEvidence={openEvidence}
          onCloseEvidence={closeEvidence}
        />
      )}
      <SinceLastReview
        catalog={catalog}
        scope={selection.scope}
        pipelineId={selection.pipeline_id}
        after={checks.state === "not_permitted" ? "ready" : checks.state}
      />
      <OverviewLinks
        dials={doors}
        sections={[
          { section: "forecast", label: t("analytics.sectionForecast") },
          { section: "pipeline", label: t("analytics.sectionPipeline") },
          { section: "performance", label: t("analytics.sectionPerformance") },
        ]}
      />
    </>
  );
}

// Under the decision and the actions: all open pipeline, today's stock by
// stage and the permitted sales by owner, and the evidence drawer for every
// figure and chart on the page, under the read each came from.
function SalesStock({
  answered,
  charts,
  evidence,
  evidenceFrom,
  onEvidence,
  onCloseEvidence,
}: Readonly<{
  answered: ReportingEvaluation;
  charts: UseQueryResult<ReportingEvaluation> | undefined;
  evidence: ReportingEvidenceRef | null;
  evidenceFrom: ReportingEvaluation | undefined;
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
  onCloseEvidence: () => void;
}>) {
  return (
    <>
      <OpenPipelineLine evaluation={answered} onEvidence={onEvidence} />
      {charts && <StockCharts charts={charts} onEvidence={onEvidence} />}
      {evidence && (
        <ReportingEvidenceDrawer
          key={JSON.stringify(evidence)}
          evaluation={evidenceFrom ?? answered}
          reference={evidence}
          onClose={onCloseEvidence}
        />
      )}
    </>
  );
}

// A pipeline the address names that is not among the reader's pipelines.
function pipelineIsGone(
  named: string | undefined,
  pipelines: readonly Pipeline[],
): boolean {
  return (
    !!named && named !== "all" && !pipelines.some(({ id }) => id === named)
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
    <Callout
      tone="info"
      kind="standing"
      title={t("analytics.overview.quarterOnlyTitle")}
      actions={
        <Button
          onClick={() =>
            onApply({ ...evaluation.selection, period: "this_quarter" })
          }
        >
          {t("reporting.compare.apply.period")}
        </Button>
      }
    >
      <p>{t("analytics.overview.quarterOnlyBody")}</p>
    </Callout>
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
