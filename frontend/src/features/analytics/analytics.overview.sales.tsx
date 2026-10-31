// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import { Button, StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { FilterBar } from "../../design-system/filterbar";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { formatDecimal, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { openAnalyticsSection } from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { usePipelines } from "../../lib/pipelines.queries";
import { problemCodeOf } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { replaceDial } from "../../lib/urlstate";
import { AttentionPanel, useAttentionList } from "./analytics.attention";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import { rangeOf } from "./analytics.forecast.landing";
import {
  MetricReading,
  metricOf,
  OverviewLinks,
  OverviewSaveActions,
  readingValue,
} from "./analytics.overview.readings";
import { SinceLastReview } from "./analytics.overview.since";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { ReportingFilters } from "./reporting.filters";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingCompactAmount,
  reportingPeriodLabel,
} from "./reporting.model";
import {
  customInterval,
  resultsThrough,
  useOverviewFilters,
  validDateRange,
} from "./reporting.overview";
import { useReportingEvaluation } from "./reporting.queries";
import { ComparisonState, gapWords } from "./reporting.targetcomparison";
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
  const filters = useOverviewFilters(pipelines, "this_quarter");
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const { metrics, blocks } = viewRequest("sales", catalog, filters.period);
  const selection: ReportingSelection = {
    scope: { kind: scope.kind, id: scope.id ?? undefined, label: scope.label },
    pipeline_id: filters.pipelineId || undefined,
    period: filters.period,
    interval: customInterval(filters.period, filters.start, filters.end, zone),
    target_basis: filters.targetBasis,
    close_window: filters.closeWindow,
    metrics,
    // The charts are Performance's; the opening page asks for figures alone.
    blocks: [],
  };
  const validDates = validDateRange(filters.period, filters.start, filters.end);
  const query = useReportingEvaluation(
    selection,
    validDates && metrics.length > 0,
  );
  const attention = useAttentionList(scope, false);
  const apply = (next: ReportingSelection) => {
    if (next.period !== filters.period) filters.setPeriod(next.period);
    if (next.pipeline_id !== selection.pipeline_id)
      filters.setPipelineId(next.pipeline_id ?? "");
    if (scopeKey(next.scope) !== scopeKey(scope))
      replaceDial("scope", scopeKey(next.scope));
    setEvidence(null);
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
          />
        }
        caption={
          validDates
            ? resultsThrough(query.data, locale, t)
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
              setEvidence(null);
            },
          }}
          onChange={(next) => {
            if (next.period !== filters.period) filters.setPeriod(next.period);
            if (next.pipeline_id !== selection.pipeline_id)
              filters.setPipelineId(next.pipeline_id ?? "");
            if (next.close_window !== filters.closeWindow)
              filters.setCloseWindow(next.close_window);
            setEvidence(null);
          }}
        />
      </FilterBar>
      {problemCodeOf(query.error) === "reporting_interval_invalid" ? (
        <ErrorLine error={query.error} />
      ) : (
        <QueryGate query={query} pendingLabel={t("analytics.overview.sales")}>
          {(evaluation) => (
            <>
              {emptyNotice(evaluation.context.scope_holds_deals)}
              <SalesReadings evaluation={evaluation} onEvidence={setEvidence} />
              <QuarterNotes
                evaluation={evaluation}
                period={filters.period}
                offersLanding={catalog.metrics.some(
                  (metric) => metric.id === "forecast_landing",
                )}
                onApply={apply}
              />
              <OpeningSummary
                sentences={salesReviewSummary(evaluation, {
                  t,
                  plural,
                  locale,
                })}
              />
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={() => setEvidence(null)}
                />
              )}
            </>
          )}
        </QueryGate>
      )}
      {/* Its sources are its own, so a failed evaluation leaves it standing. */}
      <AttentionPanel
        items={attention.items}
        state={attention.state}
        emptyLabel={t("analytics.overview.salesNothing")}
      />
      <SinceLastReview
        catalog={catalog}
        scope={selection.scope}
        pipelineId={selection.pipeline_id}
        after={attention.state}
      />
      <OverviewLinks
        sections={[
          { section: "forecast", label: t("analytics.sectionForecast") },
          { section: "pipeline", label: t("analytics.sectionPipeline") },
          { section: "performance", label: t("analytics.sectionPerformance") },
        ]}
      />
    </>
  );
}

// Sales won, the expected finish, the gap to target and the open pipeline. With
// no target to compare against, the gap's place goes to the closed win rate.
function SalesReadings({
  evaluation,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { context } = evaluation;
  const landing = metricOf(evaluation, "forecast_landing");
  const comparison = landing?.target_comparison;
  const compared =
    comparison?.status === "compared" ||
    comparison?.status === "target_reached";
  const money = (value: number | undefined) =>
    reportingCompactAmount(value, context.currency, context.currency, locale);
  const range = rangeOf(landing?.range_low_minor, landing?.range_high_minor);
  const finishBasis = comparison?.basis
    ? t("analytics.overview.finishBasis", {
        basis: t(`reporting.compare.basis.${comparison.basis}`),
      })
    : t("reporting.forecastLandingBasis");
  const open = metricOf(evaluation, "open_pipeline");
  const openCount = open?.coverage.eligible_count;
  return (
    <StatStrip label={t("analytics.overview.readings")}>
      <MetricReading
        evaluation={evaluation}
        id="bookings_won"
        detail={reportingPeriodLabel(
          context.interval,
          context.timezone,
          locale,
        )}
        onEvidence={onEvidence}
      />
      <StatCard
        narrow="row"
        label={t("analytics.overview.expectedFinish")}
        value={
          landing
            ? readingValue(landing, evaluation, t, locale)
            : t("analytics.overview.quarterOnly")
        }
        detail={
          range ? (
            <>
              <span>{finishBasis}</span>
              <span>
                {t("forecast.range", {
                  low: money(range.lowMinor),
                  high: money(range.highMinor),
                })}
              </span>
            </>
          ) : (
            finishBasis
          )
        }
        basis={range && <p>{t("forecast.rangeBasis")}</p>}
        onOpen={
          landing?.value != null
            ? () => openAnalyticsSection("forecast")
            : undefined
        }
      />
      {compared ? (
        <GapReading comparison={comparison} money={money} />
      ) : metricOf(evaluation, "closed_win_rate") &&
        (!comparison || comparison.status === "no_target") ? (
        <MetricReading
          evaluation={evaluation}
          id="closed_win_rate"
          detail={t("analytics.overview.noTargetRate")}
          onEvidence={onEvidence}
        />
      ) : (
        <NotCompared />
      )}
      <MetricReading
        evaluation={evaluation}
        id="open_pipeline"
        detail={
          comparison?.status === "compared" && comparison.coverage != null
            ? t("analytics.overview.covers", {
                ratio: formatDecimal(comparison.coverage, locale, 1),
                remaining: money(comparison.remaining),
              })
            : openCount != null
              ? plural("analytics.overview.openDeals", openCount, {
                  count: formatNumber(openCount, locale),
                })
              : undefined
        }
        onEvidence={onEvidence}
      />
    </StatStrip>
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

function GapReading({
  comparison,
  money,
}: Readonly<{
  comparison: Comparison;
  money: (value: number | undefined) => string;
}>) {
  const t = useT();
  if (comparison.status === "target_reached") {
    return (
      <StatCard
        narrow="row"
        label={t("reporting.compare.gap")}
        value={t("reporting.compare.reached")}
        detail={t("reporting.compare.remainingBasis", {
          won: money(comparison.won),
          target: money(comparison.target),
        })}
      />
    );
  }
  const { gap } = comparison;
  // A comparison without its gap names no gap, rather than a zero one.
  if (gap == null)
    return <NotCompared detail={t("analytics.overview.gapMissing")} />;
  return (
    <StatCard
      narrow="row"
      label={t("reporting.compare.gap")}
      value={money(Math.abs(gap))}
      detail={t(gapWords(gap), { target: money(comparison.target) })}
    />
  );
}

// What the quarter's comparison cannot say, and the one step that lets it.
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
  if (comparison.status === "no_target") return <TargetSetup />;
  return <ComparisonState comparison={comparison} onApply={onApply} />;
}

// Who can give the quarter a target. A reader who may not set one is told who
// can, rather than shown a button the server would refuse.
function TargetSetup() {
  const t = useT();
  const canSet = useCanWrite("sales_target", "create");
  return (
    <Callout
      tone="info"
      kind="standing"
      title={t("reporting.compare.no_target")}
      actions={
        canSet ? (
          <Button onClick={() => openAnalyticsSection("targets")}>
            {t("analytics.overview.setTarget")}
          </Button>
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
