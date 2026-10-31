// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Meter } from "../../design-system/readings";
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
  onApply,
  targetSetup,
}: Readonly<{
  evaluation: ReportingEvaluation;
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
    target = <ComparisonState comparison={comparison} onApply={onApply} />;
  return (
    <div className="analytics-hero-aside">
      {target}
      <PriorPeriod evaluation={evaluation} />
    </div>
  );
}

function TargetProgress({
  evaluation,
  comparison,
}: Readonly<{ evaluation: ReportingEvaluation; comparison: Comparison }>) {
  const t = useT();
  const { locale } = useLocale();
  const { currency } = evaluation.context;
  const money = (value: number | undefined) =>
    reportingCompactAmount(value, currency, currency, locale);
  const won = metricOf(evaluation, "bookings_won");
  // The percentage is the server's, and only when it measures the same target.
  const percent =
    won?.attainment != null && won.target === comparison.target
      ? won.attainment
      : undefined;
  const { gap } = comparison;
  return (
    <>
      {comparison.won != null && comparison.target ? (
        <Meter
          value={comparison.won}
          max={comparison.target}
          label={t("analytics.hero.attainment")}
          dense
        />
      ) : null}
      <p className="t-caption">
        {percent == null
          ? t("reporting.compare.remainingBasis", {
              won: money(comparison.won),
              target: money(comparison.target),
            })
          : t("analytics.hero.attained", {
              percent: formatNumber(Math.round(percent), locale),
              target: money(comparison.target),
            })}
      </p>
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
