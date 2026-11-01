// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { ScoreRing } from "../../design-system/standingring";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { metricOf } from "./analytics.overview.readings";
import {
  type ReportingEvaluation,
  type ReportingSelection,
  reportingCompactAmount,
} from "./reporting.model";
import { ComparisonState } from "./reporting.targetcomparison";

type Comparison = NonNullable<
  ReportingEvaluation["metrics"][number]["target_comparison"]
>;

/**
 * What stands under Sales won. It shows progress to the quarter's target,
 * what is still to win, the projected shortfall and the previous period.
 * Every figure is the server's; the page only formats.
 * A target that cannot be compared says why, with its fix, in place.
 */
export function HeroAside({
  evaluation,
  charts,
  onApply,
  targetSetup,
}: Readonly<{
  evaluation: ReportingEvaluation;
  // The chart read, once it answered: the period before is read off its trend.
  charts: ReportingEvaluation | undefined;
  onApply: (selection: ReportingSelection) => void;
  // Who can set a target, for a quarter that has none.
  targetSetup: ReactNode;
}>) {
  const comparison = metricOf(
    evaluation,
    "forecast_landing",
  )?.target_comparison;
  let target: ReactNode = null;
  if (comparison?.status === "no_target") target = targetSetup;
  else if (
    comparison?.status === "compared" ||
    comparison?.status === "target_reached"
  )
    target = <TargetProgress evaluation={evaluation} comparison={comparison} />;
  else if (comparison)
    target = (
      <ComparisonState comparison={comparison} onApply={onApply} compact />
    );
  return (
    <div className="analytics-hero-aside">
      {target}
      {charts && <PriorPeriod evaluation={charts} />}
    </div>
  );
}

/**
 * Sales won as a share of the quarter's target, with a lighter part of the
 * ring up to the expected finish. Drawn only while the two are compared.
 */
export function TargetRing({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const { locale } = useLocale();
  const comparison = compared(evaluation);
  const reading = useAttained(evaluation, comparison);
  if (!comparison || !reading || comparison.won == null || !comparison.target)
    return null;
  const share = (comparison.won / comparison.target) * 100;
  const reach =
    comparison.landing == null
      ? undefined
      : (comparison.landing / comparison.target) * 100;
  const gap = comparison.gap;
  return (
    <ScoreRing
      size="compact"
      score={share}
      reach={reach}
      display={`${formatNumber(Math.round(reading.percent ?? share), locale)}%`}
      tone={gap != null && gap > 0 ? "warning" : "success"}
      label={`${t("analytics.hero.attainment")}: ${reading.attained}`}
      caption={t("reporting.target")}
    />
  );
}

function compared(evaluation: ReportingEvaluation): Comparison | undefined {
  const comparison = metricOf(
    evaluation,
    "forecast_landing",
  )?.target_comparison;
  return comparison?.status === "compared" ||
    comparison?.status === "target_reached"
    ? comparison
    : undefined;
}

// The server's attainment, only when it measures the same target, and the
// sentence that states it.
function useAttained(
  evaluation: ReportingEvaluation,
  comparison: Comparison | undefined,
) {
  const t = useT();
  const { locale } = useLocale();
  if (!comparison) return undefined;
  const { currency } = evaluation.context;
  const money = (value: number | undefined) =>
    reportingCompactAmount(value, currency, currency, locale);
  const won = metricOf(evaluation, "bookings_won");
  const percent =
    won?.attainment != null && won.target === comparison.target
      ? won.attainment
      : undefined;
  const attained =
    percent == null
      ? t("reporting.compare.remainingBasis", {
          won: money(comparison.won),
          target: money(comparison.target),
        })
      : t("analytics.hero.attained", {
          percent: formatNumber(Math.round(percent), locale),
          target: money(comparison.target),
        });
  return { percent, attained, money };
}

function TargetProgress({
  evaluation,
  comparison,
}: Readonly<{ evaluation: ReportingEvaluation; comparison: Comparison }>) {
  const t = useT();
  const reading = useAttained(evaluation, comparison);
  if (!reading) return null;
  const { money } = reading;
  const { gap } = comparison;
  return (
    <>
      <p className="t-caption">{reading.attained}</p>
      <p className="t-caption">
        {comparison.status === "target_reached"
          ? t("reporting.compare.reached")
          : t("analytics.hero.remaining", {
              amount: money(comparison.remaining),
            })}
      </p>
      {/* A comparison without its gap names no gap, rather than a zero one. */}
      <p className="t-caption">
        {gap == null
          ? t("analytics.overview.gapMissing")
          : t(gap < 0 ? "analytics.hero.surplus" : "analytics.hero.shortfall", {
              amount: money(Math.abs(gap)),
              landing: money(comparison.landing),
            })}
      </p>
    </>
  );
}

// The same elapsed point of the period before, read off the trend's own
// comparison series. Both values are the server's. No partial period is set
// against a whole one, and nothing is subtracted here.
function PriorPeriod({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const { locale } = useLocale();
  const trend = evaluation.charts.find(
    (chart) => chart.kind === "bookings_trend",
  );
  if (trend?.coverage.status !== "ok") return null;
  const last = trend.points.at(-1);
  if (!trend.comparison_interval || last?.comparison == null)
    return <p className="t-caption">{t("analytics.hero.noPrior")}</p>;
  const { currency } = evaluation.context;
  return (
    <p className="t-caption">
      {t("analytics.hero.prior", {
        amount: reportingCompactAmount(
          last.comparison,
          trend.unit,
          currency,
          locale,
        ),
      })}
    </p>
  );
}
