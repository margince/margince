import { Button } from "../design-system/atoms";
import { PanelIntro } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  type ReportingChart,
  type ReportingEvaluation,
  reportingAmount,
} from "./reporting.model";
import { worklistLaneHref } from "./worklist.header";

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
  const amount = (value: number | null | undefined, unit: string) =>
    reportingAmount(value, unit, evaluation.context.currency, locale);
  return (
    <>
      <PanelIntro>
        {chart.state_at
          ? t("reporting.stateAt", {
              at: formatDateTime(
                chart.state_at,
                locale,
                evaluation.context.timezone,
              ),
            })
          : chart.interval
            ? t("reporting.interval", {
                start: formatDateTime(
                  chart.interval.start_at,
                  locale,
                  evaluation.context.timezone,
                ),
                end: formatDateTime(
                  new Date(Date.parse(chart.interval.end_at) - 1).toISOString(),
                  locale,
                  evaluation.context.timezone,
                ),
                zone: evaluation.context.timezone,
              })
            : evaluation.context.scope.label}
      </PanelIntro>
      {chart.kind === "stage_age" && !editionId && (
        <Button
          variant="link"
          onClick={() => {
            window.location.hash = worklistLaneHref("deals_at_risk");
          }}
        >
          {t("reporting.reviewQueue")}
        </Button>
      )}
      {chart.capture_status && (
        <p className="t-caption" role="status">
          {chart.capture_status.failure ??
            (chart.capture_status.last_success_at
              ? t("reporting.lastCapture", {
                  at: formatDateTime(
                    chart.capture_status.last_success_at,
                    locale,
                    evaluation.context.timezone,
                  ),
                })
              : t("reporting.unavailable"))}{" "}
          ·{" "}
          {t("reporting.nextRun", {
            at: formatDateTime(
              chart.capture_status.next_capture_at,
              locale,
              evaluation.context.timezone,
            ),
          })}
        </p>
      )}
      {chart.allocation_difference != null && (
        <p className="t-caption">
          {t("reporting.allocationDifference")}:{" "}
          {amount(chart.allocation_difference, chart.unit)} ·{" "}
          {t("reporting.allocatedTarget")}:{" "}
          {amount(chart.allocated_target, chart.unit)}
        </p>
      )}
      {chart.coverage.status !== "ok" && (
        <p className="t-caption">
          <Popover
            onHover
            label={t(`reporting.status.${chart.coverage.status}`)}
          >
            <p>
              {chart.coverage.reason ??
                t(`reporting.status.${chart.coverage.status}`)}
            </p>
          </Popover>
        </p>
      )}
    </>
  );
}
