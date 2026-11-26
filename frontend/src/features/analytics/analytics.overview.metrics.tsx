// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import { AppLink } from "../../design-system/applink";
import { Button, StatCard } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { Panel } from "../../design-system/panel";
import { StatStrip } from "../../design-system/statstrip";
import {
  formatAnalyticsNumber,
  formatDateAbbrev,
  formatMoneyCompact,
} from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { landingBasisLines } from "./analytics.forecast.landing";
import { formatMoneyCompactRange } from "./analytics.moneyrange";
import {
  ForecastCoverage,
  useForecastCoverage,
} from "./analytics.overview.decision";
import {
  type ChartTab,
  MetricReading,
  metricOf,
  readingValue,
} from "./analytics.overview.readings";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingCompactAmount,
} from "./reporting.model";

/** Which reading the quarter's chart shows. */
export type HeroChart = "won" | "forecast" | "range";

type QuarterProps = Parameters<typeof QuarterOverviewMetrics>[0];

/**
 * The review's head: its figures as one plate over the chart. On a quarter the
 * figures are the chart's tabs, each drawing its own reading of the period.
 */
export function SalesOverviewMetrics({
  trajectory,
  ...quarter
}: Omit<QuarterProps, "tabs"> &
  Readonly<{ trajectory?: (shown: HeroChart) => ReactNode }>) {
  const [shown, setShown] = useState<HeroChart>("won");
  if (quarter.evaluation.selection.period !== "this_quarter")
    return (
      <Panel className="analytics-sales-hero">
        <PeriodOverviewMetrics {...quarter} />
        {trajectory?.("won")}
      </Panel>
    );
  // With no chart to switch, each figure keeps its own door.
  if (!trajectory)
    return (
      <Panel className="analytics-sales-hero">
        <QuarterOverviewMetrics {...quarter} />
      </Panel>
    );
  const tab = (chart: HeroChart): ChartTab => ({
    pressed: shown === chart,
    onShow: () => setShown(chart),
  });
  return (
    <Panel className="analytics-sales-hero analytics-sales-tabs">
      <QuarterOverviewMetrics {...quarter} tabs={tab} />
      <HeroPlot shown={shown} {...quarter}>
        {trajectory?.(shown)}
      </HeroPlot>
    </Panel>
  );
}

// The chart under the tabs: what it shows, the chart, and its key. The key
// leads to the records or the page the reading came from.
function HeroPlot({
  shown,
  evaluation,
  forecastHref,
  onEvidence,
  children,
}: Readonly<{
  shown: HeroChart;
  evaluation: ReportingEvaluation;
  forecastHref: string;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  children: ReactNode;
}>) {
  const t = useT();
  const title = useId();
  const won = metricOf(evaluation, "bookings_won");
  const forecast = metricOf(evaluation, "forecast_landing");
  const goal =
    !forecast?.coverage.withheld && forecast?.target_comparison?.target != null;
  if (!children) return null;
  return (
    <section className="analytics-hero-plot" aria-labelledby={title}>
      <Heading id={title} size="small" className="analytics-hero-plot-title">
        {t(`analytics.hero.chart.${shown}`)}
      </Heading>
      {children}
      <footer className="analytics-hero-plot-foot">
        <ul className="analytics-hero-key">
          <li data-mark="line">{t("analytics.hero.key.won")}</li>
          {goal && (
            <>
              <li data-mark="pace">{t("analytics.trajectory.pace")}</li>
              <li data-mark="target">{t("analytics.hero.key.goal")}</li>
            </>
          )}
          {shown !== "won" && (
            <li data-mark="range">{t("analytics.trajectory.range")}</li>
          )}
        </ul>
        {shown === "won" ? (
          won && (
            <Button variant="link" onClick={() => onEvidence(won.evidence)}>
              {t("reporting.viewRecords")}
            </Button>
          )
        ) : (
          <AppLink href={forecastHref}>
            {t("analytics.hero.openForecast")}
          </AppLink>
        )}
      </footer>
    </section>
  );
}

function PeriodOverviewMetrics(props: Omit<QuarterProps, "tabs">) {
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
  tabs,
}: Readonly<{
  evaluation: ReportingEvaluation;
  // The one line under Sales won, and what it rests on behind its receipt.
  aside: ReactNode;
  receipt?: ReactNode;
  forecastHref: string;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  // Each figure as a tab of the chart under the row, when one is drawn.
  tabs?: (chart: HeroChart) => ChartTab;
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
    <StatStrip
      label={t("analytics.overview.readings")}
      density="plate"
      lead={!tabs}
    >
      <MetricReading
        narrow="stack"
        evaluation={evaluation}
        id="bookings_won"
        detail={aside}
        basis={receipt}
        chart={tabs?.("won")}
        onEvidence={onEvidence}
      />
      <ForecastOverviewReading
        forecast={forecast}
        evaluation={evaluation}
        chart={tabs?.("forecast")}
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
        {...(hasRange ? chartDoor(tabs?.("range"), forecastHref) : {})}
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

// A figure on a drawn chart shows itself there; with no chart it keeps its
// door to the forecast.
function chartDoor(
  chart: ChartTab | undefined,
  href: string,
): Pick<
  Parameters<typeof StatCard>[0],
  "onOpen" | "href" | "opens" | "pressed"
> {
  if (chart)
    return { onOpen: chart.onShow, pressed: chart.pressed, opens: "chart" };
  return { href, opens: "forecast" };
}

function ForecastOverviewReading({
  forecast,
  evaluation,
  chart,
  href,
  money,
}: Readonly<{
  forecast: ReportingEvaluation["metrics"][number] | undefined;
  evaluation: ReportingEvaluation;
  chart: ChartTab | undefined;
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
      {...(forecast?.value != null ? chartDoor(chart, href) : {})}
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
