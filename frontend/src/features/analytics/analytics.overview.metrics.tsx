// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { StatCard } from "../../design-system/atoms";
import { Panel } from "../../design-system/panel";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatAnalyticsNumber,
  formatDateAbbrev,
  formatMoneyCompact,
} from "../../format/format";
import { formatMoneyCompactRange } from "../../format/moneyrange";
import { useLocale, usePlural, useT } from "../../i18n";
import { landingBasisLines } from "./analytics.forecast.landing";
import {
  ForecastCoverage,
  useForecastCoverage,
} from "./analytics.overview.decision";
import {
  MetricReading,
  metricOf,
  readingValue,
} from "./analytics.overview.readings";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingCompactAmount,
} from "./reporting.model";

/**
 * The review's head: its figures as one plate, the first one leading, and on
 * a quarter the trajectory to where it can finish.
 */
export function SalesOverviewMetrics(
  props: Parameters<typeof QuarterOverviewMetrics>[0] &
    Readonly<{ trajectory?: ReactNode }>,
) {
  const { trajectory, ...quarter } = props;
  return (
    <Panel className="analytics-sales-hero">
      {props.evaluation.selection.period === "this_quarter" ? (
        <QuarterOverviewMetrics {...quarter} />
      ) : (
        <PeriodOverviewMetrics {...quarter} />
      )}
      {trajectory}
    </Panel>
  );
}

function PeriodOverviewMetrics(
  props: Parameters<typeof QuarterOverviewMetrics>[0],
) {
  const t = useT();
  return (
    <StatStrip label={t("analytics.overview.readings")} density="plate" lead>
      <MetricReading
        narrow="stack"
        evaluation={props.evaluation}
        id="bookings_won"
        onEvidence={props.onEvidence}
      />
      <MetricReading
        narrow="stack"
        evaluation={props.evaluation}
        id="open_pipeline"
        onEvidence={props.onEvidence}
      />
      <MetricReading
        narrow="stack"
        evaluation={props.evaluation}
        id="closed_win_rate"
        onEvidence={props.onEvidence}
      />
    </StatStrip>
  );
}

function QuarterOverviewMetrics({
  evaluation,
  aside,
  receipt,
  forecastHref,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  // The one line under Sales won, and what it rests on behind its receipt.
  aside: ReactNode;
  receipt?: ReactNode;
  forecastHref: string;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const coverage = useForecastCoverage(evaluation);
  const forecast = metricOf(evaluation, "forecast_landing");
  const visibleForecast = forecast?.coverage.withheld ? undefined : forecast;
  const basis = visibleForecast?.forecast_basis;
  const comparison = visibleForecast?.target_comparison;
  const money = (value: number | undefined) =>
    reportingCompactAmount(
      value,
      evaluation.context.currency,
      evaluation.context.currency,
      locale,
    );
  const periodPipeline =
    basis?.open_pipeline_minor ?? comparison?.open_pipeline;
  const hasRange =
    visibleForecast?.range_low_minor != null &&
    visibleForecast.range_high_minor != null;
  return (
    <StatStrip label={t("analytics.overview.readings")} density="plate" lead>
      <MetricReading
        narrow="stack"
        evaluation={evaluation}
        id="bookings_won"
        detail={aside}
        basis={receipt}
        onEvidence={onEvidence}
      />
      <ForecastOverviewReading
        forecast={forecast}
        evaluation={evaluation}
        href={forecastHref}
        money={money}
      />
      <StatCard
        narrow="stack"
        label={t("forecast.model.quarter")}
        value={
          hasRange
            ? // Two short amounts; the exact ends are on Forecast.
              formatMoneyCompactRange(
                visibleForecast.range_low_minor ?? 0,
                visibleForecast.range_high_minor ?? 0,
                evaluation.context.currency,
                locale,
              )
            : "—"
        }
        detail={
          hasRange ? t("forecast.model.short") : forecast?.coverage.reason
        }
        basis={hasRange ? t("forecast.model.detail") : undefined}
        href={hasRange ? forecastHref : undefined}
        opens="forecast"
      />
      <StatCard
        narrow="stack"
        label={t("analytics.outlook.periodPipeline")}
        value={periodPipeline != null ? money(periodPipeline) : "—"}
        basis={
          coverage ? <ForecastCoverage evaluation={evaluation} /> : undefined
        }
        detail={[
          basis?.open_count != null
            ? plural("analytics.outlook.dealCount", basis.open_count, {
                count: formatAnalyticsNumber(basis.open_count, locale),
              })
            : undefined,
          coverage?.summary,
        ]
          .filter(Boolean)
          .join(" · ")}
        onOpen={
          basis?.open_evidence && periodPipeline != null
            ? () => {
                if (basis.open_evidence) onEvidence(basis.open_evidence);
              }
            : undefined
        }
        opens="records"
      />
    </StatStrip>
  );
}

function ForecastOverviewReading({
  forecast,
  evaluation,
  href,
  money,
}: Readonly<{
  forecast: ReportingEvaluation["metrics"][number] | undefined;
  evaluation: ReportingEvaluation;
  href: string;
  money: (value: number | undefined) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const basis = forecast?.coverage.withheld
    ? undefined
    : forecast?.forecast_basis;
  const comparison = forecast?.coverage.withheld
    ? undefined
    : forecast?.target_comparison;
  const gap = comparison?.gap;
  const lines = landingBasisLines(basis?.landing, basis?.call_minor, money, t);
  const calledAt =
    basis?.landing.measure === "manager_call" && basis.call_at
      ? t("analytics.outlook.calledAt", {
          at: formatDateAbbrev(
            basis.call_at,
            locale,
            evaluation.context.timezone,
          ),
        })
      : undefined;
  // A landing on target has no gap to show, as on the trajectory.
  const shownGap = gap == null || gap === 0 ? undefined : gap;
  // Without a gap the first basis line is the visible detail, and only the
  // rest stays behind the receipt.
  const shown = shownGap == null ? (calledAt ?? lines.beside) : undefined;
  const folded = [calledAt, lines.beside].filter(
    (line) => line && line !== shown,
  );
  return (
    <StatCard
      narrow="stack"
      label={t(
        basis ? `forecast.basis.${basis.landing.measure}` : "forecast.landing",
      )}
      tone={lines.warning ? "warning" : undefined}
      basis={
        folded.length > 0
          ? folded.map((line) => <span key={line}>{line}</span>)
          : undefined
      }
      value={readingValue(forecast, evaluation, t, locale)}
      href={forecast?.value != null ? href : undefined}
      opens="forecast"
      detail={
        shownGap == null ? (
          shown
        ) : (
          <GapLine
            gap={shownGap}
            currency={evaluation.context.currency}
            money={money}
          />
        )
      }
    />
  );
}

// The projected gap to target in its short form; the exact amount is its title.
function GapLine({
  gap,
  currency,
  money,
}: Readonly<{
  gap: number;
  currency: string;
  money: (value: number | undefined) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const over = gap < 0;
  return (
    <span
      className={over ? undefined : "analytics-hero-short"}
      title={t(over ? "analytics.hero.surplus" : "analytics.hero.shortfall", {
        amount: money(Math.abs(gap)),
      })}
    >
      {t(over ? "analytics.hero.over" : "analytics.hero.short", {
        amount: formatMoneyCompact(Math.abs(gap), currency, locale),
      })}
    </span>
  );
}
