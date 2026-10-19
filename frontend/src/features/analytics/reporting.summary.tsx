import { StatCard } from "../../design-system/atoms";
import { StatStrip } from "../../design-system/statstrip";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT, type Words } from "../../i18n";
import {
  metricLabel,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingCompactAmount,
  reportingPeriodLabel,
} from "./reporting.model";

export function TargetProgress({
  value,
  target,
  format,
}: Readonly<{
  value: number | null;
  target?: number | null;
  format: (value: number) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (target == null || value == null) return null;
  return (
    <p className="t-caption">
      {t("reporting.remaining")}: {format(Math.max(0, target - value))}
      {target > 0
        ? ` · ${t("reporting.attainment", { percent: formatNumber(Math.round((value / target) * 100), locale) })}`
        : ""}
    </p>
  );
}

// Results over the period first, then where this quarter is heading, then
// what is open now.
const HEADLINE_METRICS = [
  "bookings_won",
  "forecast_landing",
  "open_pipeline",
  "meetings_held",
  "accepted_opportunities",
] as const;

export function ResultsSummary({
  evaluation,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const metrics = HEADLINE_METRICS.flatMap(
    (id) => evaluation.metrics.find((metric) => metric.id === id) ?? [],
  );
  if (!metrics.length) return null;
  return (
    <StatStrip>
      {metrics.map((metric) => {
        const amount = (value: number | null | undefined) =>
          reportingCompactAmount(
            value,
            metric.unit,
            evaluation.context.currency,
            locale,
          );
        const actual = metric.target_actual;
        const target = metric.target;
        const comparable = target != null && target > 0 && actual != null;
        return (
          <StatCard
            key={metric.id}
            narrow="row"
            label={metricLabel(metric.id, t)}
            value={amount(metric.value)}
            detail={
              <>
                <OutcomePeriod evaluation={evaluation} metric={metric} />
                {comparable && (
                  <span>
                    {t("reporting.targetPeriodSummary", {
                      actual: amount(actual),
                      target: amount(target),
                      percent: formatNumber(
                        Math.round((actual / target) * 100),
                        locale,
                      ),
                    })}{" "}
                    ·{" "}
                    {t(
                      actual <= target
                        ? "reporting.targetRemaining"
                        : "reporting.targetExceeded",
                      { amount: amount(Math.abs(target - actual)) },
                    )}
                    {evaluation.context.target_interval &&
                      ` · ${reportingPeriodLabel(evaluation.context.target_interval, evaluation.context.timezone, locale)}`}
                  </span>
                )}
              </>
            }
            basis={
              <>
                <p>
                  {reportingAmount(
                    metric.value,
                    metric.unit,
                    evaluation.context.currency,
                    locale,
                  )}
                </p>
                {comparable && (
                  <p>
                    {t(
                      actual <= target
                        ? "reporting.targetRemaining"
                        : "reporting.targetExceeded",
                      { amount: amount(Math.abs(target - actual)) },
                    )}
                  </p>
                )}
                <p>
                  {metric.coverage.reason ??
                    t(`reporting.status.${metric.coverage.status}`)}
                </p>
              </>
            }
            onOpen={
              metric.coverage.status === "no_data"
                ? undefined
                : () => onEvidence(metric.evidence)
            }
            meter={comparable ? { filled: actual, total: target } : undefined}
          />
        );
      })}
    </StatStrip>
  );
}

function OutcomePeriod({
  evaluation,
  metric,
}: Readonly<{
  evaluation: ReportingEvaluation;
  metric: ReportingEvaluation["metrics"][number];
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <>
      <span>
        {metric.id === "bookings_won" && metric.coverage.status === "no_data"
          ? `${t("reporting.noSalesWon")} · `
          : ""}
        {headlineBasis(evaluation, metric, { t, plural, locale })}
      </span>
      {metric.coverage.status !== "ok" &&
        !(
          metric.id === "bookings_won" && metric.coverage.status === "no_data"
        ) && <span>{t(`reporting.status.${metric.coverage.status}`)}</span>}
    </>
  );
}

// What a headline figure is about. Sales won covers the chosen period. Open
// pipeline is the state now, so it names a moment and a count. The forecast
// landing is always this fiscal quarter's.
function headlineBasis(
  evaluation: ReportingEvaluation,
  metric: ReportingEvaluation["metrics"][number],
  words: Words,
): string {
  const { t, plural, locale } = words;
  const { context } = evaluation;
  if (metric.id === "open_pipeline") {
    const count = metric.coverage.eligible_count ?? 0;
    return plural("reporting.openPipelineBasis", count, {
      count: formatNumber(count, locale),
      at: formatDateAbbrev(context.state_at, locale, context.timezone),
    });
  }
  if (metric.id === "forecast_landing") {
    return t("reporting.forecastLandingBasis");
  }
  return reportingPeriodLabel(context.interval, context.timezone, locale);
}
