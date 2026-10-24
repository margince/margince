import { Badge } from "../../design-system/atoms";
import { Popover } from "../../design-system/popover";
import { formatDateTime, formatNumber } from "../../format/format";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type ReportingChart,
  type ReportingEvaluation,
  reportingAmount,
  reportingPeriodLabel,
} from "./reporting.model";

export function ChartContext({
  chart,
  evaluation,
  editionId,
}: Readonly<{
  chart: ReportingChart;
  evaluation: ReportingEvaluation;
  editionId?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { timezone, currency } = evaluation.context;
  return (
    <div className="reporting-context">
      <span className="t-caption">
        {contextPeriod(chart, evaluation, editionId, locale, t)}
      </span>
      {chart.kind === "owner_attainment" && (
        <OwnerAttainmentNotes
          chart={chart}
          evaluation={evaluation}
          captured={editionId != null}
        />
      )}
      {(chart.kind === "stage_distribution" || chart.kind === "stage_age") && (
        <span className="t-caption">
          {t("reporting.closeWindow")}:{" "}
          {evaluation.context.close_interval
            ? reportingPeriodLabel(
                evaluation.context.close_interval,
                timezone,
                locale,
              )
            : t("reporting.all_open")}
        </span>
      )}
      <Popover
        onHover
        className="evmark-trigger t-caption"
        label={t("reporting.contextDetails")}
      >
        <p>
          {evaluation.context.scope.label} · {timezone}
        </p>
        <p>
          {formatDateTime(
            chart.state_at ?? evaluation.context.evaluated_at,
            locale,
            timezone,
          )}
        </p>
        {chart.interval && (
          <p>
            {formatDateTime(chart.interval.start_at, locale, timezone)} –{" "}
            {formatDateTime(
              new Date(Date.parse(chart.interval.end_at) - 1).toISOString(),
              locale,
              timezone,
            )}
          </p>
        )}
        {chart.capture_status && (
          <p>
            {t("reporting.lastCapture", {
              at: chart.capture_status.last_success_at
                ? formatDateTime(
                    chart.capture_status.last_success_at,
                    locale,
                    timezone,
                  )
                : "—",
            })}{" "}
            ·{" "}
            {t("reporting.nextRun", {
              at: formatDateTime(
                chart.capture_status.next_capture_at,
                locale,
                timezone,
              ),
            })}
          </p>
        )}
        {chart.allocation_difference != null && (
          <p>
            {t(
              chart.allocation_difference < 0
                ? "reporting.overallocated"
                : "reporting.unallocated",
            )}
            :{" "}
            {reportingAmount(
              Math.abs(chart.allocation_difference),
              chart.unit,
              currency,
              locale,
            )}
          </p>
        )}
        {chart.coverage.status !== "ok" && chart.coverage.reason && (
          <p>{chart.coverage.reason}</p>
        )}
      </Popover>
      {chart.kind === "stage_age" && <SampleDetails chart={chart} />}
      {/* Coverage short of complete is said beside the period, where it
          qualifies the figure; the reason joins the details above. */}
      {chart.coverage.status !== "ok" && (
        <Badge tone="warning">
          {t(`reporting.status.${chart.coverage.status}`)}
        </Badge>
      )}
      {chart.capture_status?.failure && (
        <p className="t-caption" role="status">
          {chart.capture_status.failure}
        </p>
      )}
    </div>
  );
}

// How many deals each stage's durations were read from, beside the chart's
// other details rather than under the chart it qualifies.
function SampleDetails({ chart }: Readonly<{ chart: ReportingChart }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <Popover
      onHover
      className="evmark-trigger t-caption"
      label={t("reporting.sampleDetails")}
    >
      {chart.points.map((point) => (
        <p key={point.key} className="t-caption">
          {point.label} ·{" "}
          {point.observations == null
            ? t("reporting.observationsUnavailable")
            : plural("reporting.observations", point.observations, {
                count: formatNumber(point.observations, locale),
              })}
          {point.value == null
            ? ` · ${t("reporting.status.insufficient_sample")}`
            : ""}
        </p>
      ))}
    </Popover>
  );
}

function contextPeriod(
  chart: ReportingChart,
  evaluation: ReportingEvaluation,
  editionId: string | undefined,
  locale: Locale,
  t: Translator,
): string | undefined {
  const timezone = evaluation.context.timezone;
  return chart.interval
    ? reportingPeriodLabel(chart.interval, timezone, locale)
    : chart.state_at
      ? editionId
        ? formatDateTime(chart.state_at, locale, timezone)
        : t(
            chart.kind === "stage_distribution" || chart.kind === "stage_age"
              ? "reporting.currentPipeline"
              : "reporting.currentState",
          )
      : evaluation.context.scope.label;
}

// Whom an owner chart credits. The rules differ by metric, and a ranking
// without them reads as one uniform measure of each owner's work.
// A saved edition keeps the owner it captured, so its sales are credited to
// that owner rather than to today's.
const OWNER_CREDIT: Partial<Record<ReportingChart["metric"], MessageKey>> = {
  bookings_won: "reporting.creditCurrentOwner",
  qualified_pipeline_created: "reporting.creditQualifyingOwner",
};
const CAPTURED_OWNER_CREDIT: Partial<
  Record<ReportingChart["metric"], MessageKey>
> = {
  bookings_won: "reporting.creditCapturedOwner",
  qualified_pipeline_created: "reporting.creditQualifyingOwner",
};

// An actual to date reads as short of a whole-period target until the period
// ends. So the share of the period already gone is said.
function OwnerAttainmentNotes({
  chart,
  evaluation,
  captured,
}: Readonly<{
  chart: ReportingChart;
  evaluation: ReportingEvaluation;
  captured: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const credit = (captured ? CAPTURED_OWNER_CREDIT : OWNER_CREDIT)[
    chart.metric
  ];
  // Only where the server applied a target to these points. A week read
  // against a month's target carries none. The month's progress would then
  // describe a period the chart does not show.
  const target = chart.points.some((point) => point.target != null)
    ? evaluation.context.target_interval
    : undefined;
  const elapsed = target
    ? elapsedShare(target, evaluation.context.evaluated_at)
    : null;
  return (
    <>
      {credit && <span className="t-caption">{t(credit)}</span>}
      {elapsed != null && (
        <span className="t-caption">
          {t("reporting.targetPeriodElapsed", {
            percent: formatNumber(Math.round(elapsed * 100), locale),
          })}
        </span>
      )}
    </>
  );
}

function elapsedShare(
  interval: { start_at: string; end_at: string },
  at: string,
): number | null {
  const start = Date.parse(interval.start_at);
  const end = Date.parse(interval.end_at);
  const now = Date.parse(at);
  if (!(end > start) || Number.isNaN(now)) {
    return null;
  }
  return Math.min(1, Math.max(0, (now - start) / (end - start)));
}
