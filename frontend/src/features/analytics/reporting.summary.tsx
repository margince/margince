import { StatCard } from "../../design-system/atoms";
import { StatStrip } from "../../design-system/statstrip";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT, type Words } from "../../i18n";
import { openAnalyticsSection } from "../../lib/analytics.address";
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
// what is open now. Each view shows the ones it read.
const HEADLINE_METRICS = [
  "bookings_won",
  "closed_win_rate",
  "qualified_pipeline_created",
  "forecast_landing",
  "open_pipeline",
  "meetings_held",
  "accepted_opportunities",
  "leads_created",
  "leads_promoted",
  "lead_first_response",
  "partner_pipeline",
  "partner_won",
  "partner_commission",
  "partner_activity",
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
            onOpen={headlineOpen(metric, onEvidence)}
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
        {/* Out of nothing says no more than the status beside it. */}
        {!metric.denominator
          ? ""
          : `${t("reporting.outOf", { total: formatNumber(metric.denominator, locale) })} · `}
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
    const at = formatDateAbbrev(context.state_at, locale, context.timezone);
    const count = metric.coverage.eligible_count;
    // A withheld count stays unknown rather than reading as zero deals.
    return count == null
      ? t("reporting.openPipelineAsOf", { at })
      : plural("reporting.openPipelineBasis", count, {
          count: formatNumber(count, locale),
          at,
        });
  }
  if (metric.id === "forecast_landing") {
    return t("reporting.forecastLandingBasis");
  }
  // A figure read as things stand names the moment, not a period.
  if (metric.evidence.context_id === "state") {
    return t("reporting.openPipelineAsOf", {
      at: formatDateAbbrev(context.state_at, locale, context.timezone),
    });
  }
  return reportingPeriodLabel(context.interval, context.timezone, locale);
}

// The forecast landing is a projection, not a sum of records. Its evidence is
// the Forecast section, where each part opens its own deals.
function headlineOpen(
  metric: ReportingEvaluation["metrics"][number],
  onEvidence: (reference: ReportingEvidenceRef) => void,
): (() => void) | undefined {
  if (metric.coverage.status === "no_data") {
    return undefined;
  }
  if (metric.id === "forecast_landing") {
    return () => openAnalyticsSection("forecast");
  }
  return () => onEvidence(metric.evidence);
}
