import type { Translator } from "../../i18n";
import {
  blockLabel,
  type MetricNames,
  type ReportingChart,
  type ReportingEvaluation,
} from "./reporting.model";

export function chartTitle(
  chart: ReportingChart,
  evaluation: ReportingEvaluation,
  t: Translator,
  metricName: MetricNames,
): string {
  if (chart.kind === "owner_attainment")
    return t("reporting.ownerMetric", { metric: metricName(chart.metric) });
  // A chart named after its metric, as stage age is, reads the catalog's name.
  if (chart.kind === "target_progress" || chart.kind === chart.metric)
    return metricName(chart.metric);
  if (chart.kind === "breakdown")
    return t("reporting.breakdownTitle", {
      metric: metricName(chart.metric),
    });
  if (
    chart.kind === "sdr_outcomes" &&
    !evaluation.charts.some(
      (other) => other.kind === "sdr_outcomes" && other.metric !== chart.metric,
    )
  )
    return metricName(chart.metric);
  return blockLabel(chart.kind, t);
}
