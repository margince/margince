// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Meter } from "../../design-system/readings";
import { formatAnalyticsNumber } from "../../format/format";
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
 * The one line under Sales won: the target it is read against. A target that
 * cannot be compared says why, with its fix, in place.
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
  const t = useT();
  const { locale } = useLocale();
  const forecast = metricOf(evaluation, "forecast_landing");
  if (forecast?.coverage.withheld) return null;
  const comparison = forecast?.target_comparison;
  if (comparison?.status === "no_target") return targetSetup;
  if (
    comparison?.status === "compared" ||
    comparison?.status === "target_reached"
  ) {
    const { currency } = evaluation.context;
    return (
      <span>
        {comparison.status === "target_reached"
          ? t("reporting.compare.reached")
          : t("analytics.hero.ofTarget", {
              target: reportingCompactAmount(
                comparison.target,
                currency,
                currency,
                locale,
              ),
            })}
      </span>
    );
  }
  return comparison ? (
    <ComparisonState comparison={comparison} onApply={onApply} compact />
  ) : null;
}

/**
 * What Sales won rests on, folded behind its receipt. It holds progress to the
 * target, what is still to win and the same point of the period before.
 * Every figure is the server's; the page only formats.
 */
// Nothing when there is nothing to fold, so the reading shows no receipt.
export function heroReceipt(
  evaluation: ReportingEvaluation,
  // The chart read, once it answered: the period before is read off its trend.
  charts: ReportingEvaluation | undefined,
): ReactNode {
  const forecast = metricOf(evaluation, "forecast_landing");
  const comparison = forecast?.coverage.withheld
    ? undefined
    : forecast?.target_comparison;
  const compared =
    comparison?.status === "compared" || comparison?.status === "target_reached"
      ? comparison
      : undefined;
  const prior = priorOf(charts);
  if (!compared && !prior) return undefined;
  return (
    <div className="analytics-hero-aside">
      {compared && (
        <TargetProgress evaluation={evaluation} comparison={compared} />
      )}
      {charts && prior && <PriorPeriod evaluation={charts} prior={prior} />}
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
              percent: formatAnalyticsNumber(Math.round(percent), locale),
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
    </>
  );
}

// The same elapsed point of the period before, read off the trend's own
// comparison series. Both values are the server's. No partial period is set
// against a whole one, and nothing is subtracted here.
function priorOf(
  evaluation: ReportingEvaluation | undefined,
): Readonly<{ value: number; unit: string }> | undefined {
  const trend = evaluation?.charts.find(
    (chart) => chart.kind === "bookings_trend",
  );
  if (trend?.coverage.status !== "ok" || !trend.comparison_interval)
    return undefined;
  const last = trend.points.at(-1)?.comparison;
  return last == null ? undefined : { value: last, unit: trend.unit };
}

function PriorPeriod({
  evaluation,
  prior,
}: Readonly<{
  evaluation: ReportingEvaluation;
  prior: Readonly<{ value: number; unit: string }>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { currency } = evaluation.context;
  return (
    <p className="t-caption">
      {t("analytics.hero.prior", {
        amount: reportingCompactAmount(
          prior.value,
          prior.unit,
          currency,
          locale,
        ),
      })}
    </p>
  );
}
