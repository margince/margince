import { Badge } from "../design-system/atoms";
import { Popover } from "../design-system/popover";
import { formatDateTime } from "../format/format";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
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
      <Popover onHover label={t("reporting.contextDetails")}>
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
