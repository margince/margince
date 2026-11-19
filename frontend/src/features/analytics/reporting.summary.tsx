import type { ComponentProps } from "react";
import { StatCard } from "../../design-system/atoms";
import { StatStrip } from "../../design-system/statstrip";
import { formatAnalyticsNumber, formatDateAbbrev } from "../../format/format";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
  type Words,
} from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { scopeKey } from "./analytics.context";
import {
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingCompactAmount,
  reportingPeriodLabel,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";

type Metric = ReportingEvaluation["metrics"][number];

/**
 * How far a target chart's metric stands from its target, as the server
 * states it. A chart held against no target says nothing.
 */
export function TargetProgress({
  chart,
  evaluation,
  format,
}: Readonly<{
  chart: ReportingChart;
  evaluation: ReportingEvaluation;
  format: (value: number) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const metric = evaluation.metrics.find((each) => each.id === chart.metric);
  const held = chart.points.some((point) => point.target != null);
  if (!held || metric?.target_delta == null) return null;
  return (
    <p className="t-caption">
      {t("reporting.remaining")}: {format(Math.max(0, -metric.target_delta))}
      {metric.attainment != null
        ? ` · ${t("reporting.attainment", { percent: formatAnalyticsNumber(Math.round(metric.attainment), locale) })}`
        : ""}
    </p>
  );
}

// The target sentences of a headline figure, from the delta and share the
// server worked out. A figure with no positive target has no share to read.
function targetLines(
  metric: Metric,
  amount: (value: number | null | undefined) => string,
  t: Translator,
  locale: Locale,
) {
  const { target, target_actual: actual, target_delta: delta } = metric;
  if (
    target == null ||
    actual == null ||
    delta == null ||
    metric.attainment == null
  )
    return null;
  return {
    summary: t("reporting.targetPeriodSummary", {
      actual: amount(actual),
      target: amount(target),
      percent: formatAnalyticsNumber(Math.round(metric.attainment), locale),
    }),
    gap: t(
      delta <= 0 ? "reporting.targetRemaining" : "reporting.targetExceeded",
      {
        amount: amount(Math.abs(delta)),
      },
    ),
    meter: { filled: actual, total: target },
  };
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
  "leads_qualified",
  "leads_promoted",
  "lead_first_response",
  "leads_awaiting_response",
  "leads_awaiting_response_now",
  "partner_pipeline",
  "partner_won",
  "partner_commission",
  "partner_activity",
] as const;

export function ResultsSummary({
  evaluation,
  density,
  lead,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  density?: ComponentProps<typeof StatStrip>["density"];
  // The first figure leads the plate, as on Overview.
  lead?: boolean;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const { locale } = useLocale();
  const metrics = HEADLINE_METRICS.flatMap(
    (id) => evaluation.metrics.find((metric) => metric.id === id) ?? [],
  );
  if (!metrics.length) return null;
  return (
    <StatStrip density={density} lead={lead}>
      {metrics.map((metric) => {
        const amount = (value: number | null | undefined) =>
          reportingCompactAmount(
            value,
            metric.unit,
            evaluation.context.currency,
            locale,
          );
        const lines = targetLines(metric, amount, t, locale);
        return (
          <StatCard
            key={metric.id}
            narrow="row"
            label={metricName(metric.id)}
            value={amount(metric.value)}
            detail={
              <>
                <OutcomePeriod evaluation={evaluation} metric={metric} />
                {lines && (
                  <span>
                    {lines.summary} · {lines.gap}
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
                {lines && <p>{lines.gap}</p>}
                <p>
                  {metric.coverage.reason ??
                    t(`reporting.status.${metric.coverage.status}`)}
                </p>
              </>
            }
            onOpen={headlineOpen(metric, onEvidence)}
            href={
              metric.id === "forecast_landing" &&
              metric.coverage.status !== "no_data"
                ? forecastHref(evaluation.selection)
                : undefined
            }
            meter={lines?.meter}
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
          : `${t("reporting.outOf", { total: formatAnalyticsNumber(metric.denominator, locale) })} · `}
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
// landing is always this fiscal quarter's, on the basis it was projected with.
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
          count: formatAnalyticsNumber(count, locale),
          at,
        });
  }
  // An edition stored before the basis was kept says the basis is unknown.
  if (metric.id === "forecast_landing") {
    const landing = metric.forecast_basis?.landing;
    const basis = landing
      ? t(`forecast.basis.${landing.measure}`)
      : t("forecast.basis.unknown");
    return `${t("reporting.forecastLandingBasis")} · ${basis}`;
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
// the Forecast section, where each part opens its own deals: a link that
// carries this selection's scope, pipeline and period.
function forecastHref(selection: ReportingEvaluation["selection"]): string {
  return analyticsSectionHref(
    "forecast",
    new Map([
      ["scope", scopeKey(selection.scope)],
      ["pipeline", selection.pipeline_id ?? "all"],
      ["period", selection.period],
    ]),
  );
}

function headlineOpen(
  metric: ReportingEvaluation["metrics"][number],
  onEvidence: (reference: ReportingEvidenceRef) => void,
): (() => void) | undefined {
  if (metric.coverage.status === "no_data") {
    return undefined;
  }
  if (metric.id === "forecast_landing") return undefined;
  return () => onEvidence(metric.evidence);
}
