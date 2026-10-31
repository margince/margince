// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Button, StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { FilterBar } from "../../design-system/filterbar";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatDateAbbrev,
  formatDecimal,
  formatNumber,
} from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { usePipelines } from "../../lib/pipelines.queries";
import { problemCodeOf } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { replaceDial, type UrlParams, useUrlParams } from "../../lib/urlstate";
import { AttentionPanel, useAttentionList } from "./analytics.attention";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import {
  landingBasisLines,
  ModelRangeCard,
  rangeOf,
} from "./analytics.forecast.landing";
import {
  BoardChart,
  SalesHero,
  salesBoardBlocks,
} from "./analytics.overview.board";
import {
  type OverviewDials,
  outboundDials,
  scopeDials,
} from "./analytics.overview.context";
import {
  MetricReading,
  metricOf,
  OverviewLinks,
  OverviewSaveActions,
  readingValue,
} from "./analytics.overview.readings";
import { SinceLastReview } from "./analytics.overview.since";
import { periodCaption } from "./analytics.period";
import {
  ReportingEvidenceDrawer,
  useSelectionEvidence,
} from "./reporting.evidence";
import { ReportingFilters } from "./reporting.filters";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingCompactAmount,
} from "./reporting.model";
import {
  customInterval,
  useOverviewFilters,
  validDateRange,
} from "./reporting.overview";
import { useReportingEvaluation } from "./reporting.queries";
import { ComparisonState } from "./reporting.targetcomparison";
import { viewRequest } from "./reporting.views";
import { OpeningSummary, salesReviewSummary } from "./reporting.written";

type Catalog = components["schemas"]["ReportingCatalog"];
type Pipeline = components["schemas"]["Pipeline"];
type Comparison = NonNullable<
  ReportingEvaluation["metrics"][number]["target_comparison"]
>;

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
  const plural = usePlural();
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
    blocks: salesBoardBlocks(blocks),
  };
  const validDates = validDateRange(filters.period, filters.start, filters.end);
  // A pipeline the address names and this reader cannot open is said, never
  // swapped for the default one.
  const named = params.get("pipeline");
  const pipelineGone =
    !!named && named !== "all" && !pipelines.some(({ id }) => id === named);
  const query = useReportingEvaluation(
    selection,
    validDates && metrics.length > 0 && !pipelineGone,
  );
  const attention = useAttentionList(scope, false);
  const { evidence, openEvidence, closeEvidence } = useSelectionEvidence(
    JSON.stringify(selection),
  );
  const doors = outboundDials(
    "sales",
    answeredDials(selection, filters.start, filters.end),
  );
  const apply = (next: ReportingSelection) => {
    if (next.period !== filters.period) filters.setPeriod(next.period);
    if (next.pipeline_id !== selection.pipeline_id)
      filters.setPipelineId(next.pipeline_id ?? "");
    if (scopeKey(next.scope) !== scopeKey(scope))
      replaceDial("scope", scopeKey(next.scope));
  };
  return (
    <>
      <FilterBar
        label={t("analytics.filters")}
        actions={
          <OverviewSaveActions
            selection={
              query.data ? { ...query.data.selection, blocks } : undefined
            }
            scope={scopeKey(scope)}
          />
        }
        caption={
          validDates
            ? periodCaption(query.data, locale, t)
            : t("reporting.chooseDates")
        }
      >
        {switcher}
        {scopeControl}
        <ReportingFilters
          selection={selection}
          showScope={false}
          showTargets={false}
          dates={{
            from: filters.start,
            through: filters.end,
            onChange: (from, through) => {
              filters.setStart(from);
              filters.setEnd(through);
            },
          }}
          onChange={(next) => {
            if (next.period !== filters.period) filters.setPeriod(next.period);
            if (next.pipeline_id !== selection.pipeline_id)
              filters.setPipelineId(next.pipeline_id ?? "");
            if (next.close_window !== filters.closeWindow)
              filters.setCloseWindow(next.close_window);
          }}
        />
      </FilterBar>
      {pipelineGone ? (
        <PipelineGone
          defaultPipeline={
            pipelines.find((pipeline) => pipeline.is_default) ?? pipelines[0]
          }
        />
      ) : problemCodeOf(query.error) === "reporting_interval_invalid" ? (
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
                targetsHref={analyticsSectionHref(
                  "targets",
                  scopeDials(scopeKey(scope)),
                )}
              />
              <SalesHero evaluation={evaluation} onEvidence={openEvidence} />
              <SalesReadings
                evaluation={evaluation}
                onEvidence={openEvidence}
                forecastHref={analyticsSectionHref("forecast", doors)}
              />
              <div className="analytics-pair">
                <BoardChart
                  evaluation={evaluation}
                  kind="stage_distribution"
                  onEvidence={openEvidence}
                />
                <BoardChart
                  evaluation={evaluation}
                  kind="owner_attainment"
                  onEvidence={openEvidence}
                />
              </div>
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={closeEvidence}
                />
              )}
            </>
          )}
        </QueryGate>
      )}
      <div className="analytics-pair">
        {/* Its sources are its own, so a failed evaluation leaves it standing. */}
        <AttentionPanel
          items={attention.items}
          state={attention.state}
          emptyLabel={t("analytics.overview.salesNothing")}
          dials={findingDials(doors)}
        />
        {query.data && (
          <OpeningSummary
            title={t("analytics.overview.summary")}
            sentences={salesReviewSummary(query.data, { t, plural, locale })}
          />
        )}
      </div>
      <SinceLastReview
        catalog={catalog}
        scope={selection.scope}
        pipelineId={selection.pipeline_id}
        after={attention.state}
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

// The expected finish with its basis, and the modelled finish beside it. Then
// the shortfall, coverage with both operands, the win rate and open pipeline.
// Every figure is the server's; the page only formats.
function SalesReadings({
  evaluation,
  onEvidence,
  forecastHref,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  forecastHref: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { context } = evaluation;
  const landing = metricOf(evaluation, "forecast_landing");
  const comparison = landing?.target_comparison;
  const money = (value: number | undefined) =>
    reportingCompactAmount(value, context.currency, context.currency, locale);
  const open = metricOf(evaluation, "open_pipeline");
  const openCount = open?.coverage.eligible_count;
  return (
    <StatStrip label={t("analytics.overview.readings")}>
      <ExpectedFinish
        evaluation={evaluation}
        money={money}
        forecastHref={forecastHref}
      />
      {landing?.value != null && (
        <ModelRangeCard
          label={t("forecast.model.quarter")}
          range={rangeOf(landing.range_low_minor, landing.range_high_minor)}
          wonMinor={landing.forecast_basis?.landing.won_minor}
          unpriced={unpricedOf(landing.forecast_basis)}
          fxMissing={landing.forecast_basis?.fx_missing_count ?? 0}
          money={money}
        />
      )}
      {comparison && <TargetReadings comparison={comparison} money={money} />}
      {metricOf(evaluation, "closed_win_rate") && (
        <MetricReading
          evaluation={evaluation}
          id="closed_win_rate"
          detail={t("analytics.overview.winRateBasis")}
          onEvidence={onEvidence}
        />
      )}
      <MetricReading
        evaluation={evaluation}
        id="open_pipeline"
        // Never the coverage numerator: that counts only the deals eligible
        // to close in the quarter.
        label={t(
          context.close_interval
            ? "analytics.overview.openClosing"
            : "analytics.overview.allOpen",
        )}
        // Today's stock, dated, so a past period never reads it as its own.
        detail={
          openCount != null
            ? plural("reporting.openPipelineBasis", openCount, {
                count: formatNumber(openCount, locale),
                at: formatDateAbbrev(
                  context.state_at,
                  locale,
                  context.timezone,
                ),
              })
            : t("reporting.openPipelineAsOf", {
                at: formatDateAbbrev(
                  context.state_at,
                  locale,
                  context.timezone,
                ),
              })
        }
        onEvidence={onEvidence}
      />
    </StatStrip>
  );
}

type Basis = NonNullable<
  ReportingEvaluation["metrics"][number]["forecast_basis"]
>;

function unpricedOf(basis: Basis | undefined): number {
  return basis ? basis.eligible_count - basis.priced_count : 0;
}

// The landing and the two lines that say what it counts. A figure with no
// basis says it is unknown rather than borrowing one.
function ExpectedFinish({
  evaluation,
  money,
  forecastHref,
}: Readonly<{
  evaluation: ReportingEvaluation;
  money: (value: number | undefined) => string;
  forecastHref: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const landing = metricOf(evaluation, "forecast_landing");
  if (!landing)
    return (
      <StatCard
        narrow="row"
        label={t("analytics.overview.expectedFinish")}
        value={t("analytics.overview.quarterOnly")}
      />
    );
  const basis = landing.forecast_basis;
  const lines =
    landing.value == null
      ? undefined
      : landingBasisLines(basis?.landing, basis?.call_minor, money, t);
  return (
    <StatCard
      narrow="row"
      label={t("analytics.overview.expectedFinish")}
      value={readingValue(landing, evaluation, t, locale)}
      tone={lines?.warning ? "warning" : undefined}
      detail={
        lines && (
          <>
            <span>{lines.basis}</span>
            {lines.beside && <span>{lines.beside}</span>}
          </>
        )
      }
      href={landing.value != null ? forecastHref : undefined}
    />
  );
}

// The quarter against its target: the projected shortfall (target minus the
// landing) and coverage of what is still to win (target minus won). The two
// are different figures, and a state that compares nothing says why.
function TargetReadings({
  comparison,
  money,
}: Readonly<{
  comparison: Comparison;
  money: (value: number | undefined) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (comparison.status === "no_target") return null;
  if (
    comparison.status !== "compared" &&
    comparison.status !== "target_reached"
  )
    return <NotCompared />;
  const { gap } = comparison;
  return (
    <>
      {/* A comparison without its gap names no gap, rather than a zero one. */}
      {gap == null ? (
        <NotCompared detail={t("analytics.overview.gapMissing")} />
      ) : (
        <StatCard
          narrow="row"
          label={t(
            gap < 0
              ? "analytics.overview.surplus"
              : "analytics.overview.shortfall",
          )}
          value={money(Math.abs(gap))}
          detail={t("analytics.overview.shortfallOperands", {
            target: money(comparison.target),
            landing: money(comparison.landing),
          })}
        />
      )}
      {comparison.status === "target_reached" ? (
        <StatCard
          narrow="row"
          label={t("analytics.overview.coverage")}
          value={t("reporting.compare.reached")}
          detail={t("reporting.compare.remainingBasis", {
            won: money(comparison.won),
            target: money(comparison.target),
          })}
        />
      ) : comparison.coverage == null ? (
        <NotCompared />
      ) : (
        <StatCard
          narrow="row"
          label={t("analytics.overview.coverage")}
          value={t("reporting.compare.ratio", {
            ratio: formatDecimal(comparison.coverage, locale, 1),
          })}
          detail={t("reporting.compare.coverageBasis", {
            open: money(comparison.open_pipeline),
            remaining: money(comparison.remaining),
          })}
        />
      )}
    </>
  );
}

function NotCompared({ detail }: Readonly<{ detail?: string }>) {
  const t = useT();
  return (
    <StatCard
      narrow="row"
      label={t("reporting.compare.gap")}
      value={t("analytics.overview.notCompared")}
      detail={detail ?? t("analytics.overview.notComparedDetail")}
    />
  );
}

// What the quarter's comparison cannot say, and the one step that lets it.
function QuarterNotes({
  evaluation,
  period,
  offersLanding,
  onApply,
  targetsHref,
}: Readonly<{
  evaluation: ReportingEvaluation;
  period: ReportingSelection["period"];
  offersLanding: boolean;
  onApply: (selection: ReportingSelection) => void;
  targetsHref: string;
}>) {
  const t = useT();
  const comparison = metricOf(
    evaluation,
    "forecast_landing",
  )?.target_comparison;
  if (!comparison) {
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
  if (
    comparison.status === "compared" ||
    comparison.status === "target_reached"
  )
    return null;
  if (comparison.status === "no_target")
    return <TargetSetup href={targetsHref} />;
  return <ComparisonState comparison={comparison} onApply={onApply} />;
}

// Who can give the quarter a target. A reader who may not set one is told who
// can, rather than shown a button the server would refuse. `href` opens the
// targets for the scope the comparison was about.
function TargetSetup({ href }: Readonly<{ href: string }>) {
  const t = useT();
  const canSet = useCanWrite("sales_target", "create");
  return (
    <Callout
      tone="info"
      kind="standing"
      title={t("reporting.compare.no_target")}
      actions={
        canSet ? (
          <AppLink className="btn btn-ghost" href={href}>
            {t("analytics.overview.setTarget")}
          </AppLink>
        ) : undefined
      }
    >
      <p>
        {t(
          canSet
            ? "analytics.overview.setTargetBody"
            : "analytics.overview.targetWho",
        )}
      </p>
    </Callout>
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
