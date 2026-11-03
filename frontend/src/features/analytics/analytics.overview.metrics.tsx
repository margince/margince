// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { StatCard } from "../../design-system/atoms";
import { StatStrip } from "../../design-system/statstrip";
import { formatDateAbbrev, formatNumber } from "../../format/format";
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

export function SalesOverviewMetrics({
  evaluation,
  aside,
  forecastHref,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  aside: ReactNode;
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
  const quarter = evaluation.selection.period === "this_quarter";
  const hasRange =
    visibleForecast?.range_low_minor != null &&
    visibleForecast.range_high_minor != null;
  return (
    <StatStrip label={t("analytics.overview.readings")} density="compact">
      <MetricReading
        evaluation={evaluation}
        id="bookings_won"
        detail={aside}
        onEvidence={onEvidence}
      />
      <ForecastOverviewReading
        forecast={forecast}
        evaluation={evaluation}
        href={forecastHref}
        money={money}
      />
      <StatCard
        narrow="row"
        label={t("forecast.model.quarter")}
        value={
          hasRange
            ? `${money(visibleForecast.range_low_minor)} – ${money(visibleForecast.range_high_minor)}`
            : "—"
        }
        detail={
          hasRange ? t("forecast.model.detail") : forecast?.coverage.reason
        }
        href={hasRange ? forecastHref : undefined}
      />
      <StatCard
        narrow="row"
        label={t("analytics.outlook.periodPipeline")}
        value={quarter && periodPipeline != null ? money(periodPipeline) : "—"}
        basis={
          quarter && coverage ? (
            <ForecastCoverage evaluation={evaluation} />
          ) : undefined
        }
        detail={
          quarter && basis ? (
            <>
              <span>
                {plural("analytics.outlook.dealCount", basis.eligible_count, {
                  count: formatNumber(basis.eligible_count, locale),
                })}
              </span>
              {coverage?.summary && <span>{coverage.summary}</span>}
            </>
          ) : undefined
        }
        href={quarter && periodPipeline != null ? forecastHref : undefined}
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
  return (
    <StatCard
      narrow="row"
      label={t(
        basis ? `forecast.basis.${basis.landing.measure}` : "forecast.landing",
      )}
      tone={lines.warning ? "warning" : undefined}
      basis={calledAt && lines.beside ? <span>{lines.beside}</span> : undefined}
      value={readingValue(forecast, evaluation, t, locale)}
      href={forecast?.value != null ? href : undefined}
      detail={
        <>
          {(calledAt || lines.beside) && (
            <span>{calledAt || lines.beside}</span>
          )}
          {gap != null && (
            <span>
              {t(
                gap < 0 ? "analytics.hero.surplus" : "analytics.hero.shortfall",
                {
                  amount: money(Math.abs(gap)),
                },
              )}
            </span>
          )}
        </>
      }
    />
  );
}
