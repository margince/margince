import { Button, Disclosure, EmptyState } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { SegmentBar } from "../../design-system/readings";
import {
  BulletChart,
  CumulativeChart,
  RangeChart,
} from "../../design-system/report-charts";
import { Waterfall } from "../../design-system/waterfall";
import { formatDateTime, formatMoneyCompact } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import { CohortTable } from "./reporting.cohort";
import {
  blockLabel,
  type MetricNames,
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingCompactAmount,
  reportingMoneyUnit,
} from "./reporting.model";
import { movementLabel } from "./reporting.movement";
import { useMetricNames } from "./reporting.queries";
import { SdrOutcomes } from "./reporting.sdroutcomes";
import { TargetProgress } from "./reporting.summary";
import "./reporting.css";

type EvidenceAction = (reference: ReportingEvidenceRef) => void;

import { Breakdown, StageDistribution } from "./reporting.groups";
export function ChartBody({
  chart,
  evaluation,
  onEvidence,
  trendSize,
}: Readonly<{
  chart: ReportingChart;
  evaluation: ReportingEvaluation;
  onEvidence: EvidenceAction;
  /** `compact` is a shorter trend, for one beside a lead figure. */
  trendSize?: "default" | "compact";
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const { locale } = useLocale();
  const format = (value: number | null | undefined) =>
    reportingAmount(value, chart.unit, evaluation.context.currency, locale);
  const compact = (value: number) =>
    reportingMoneyUnit(chart.unit, evaluation.context.currency)
      ? formatMoneyCompact(value, evaluation.context.currency, locale)
      : format(value);
  const readings = chart.points.map((point) => ({
    ...point,
    amount: reportingCompactAmount(
      point.value,
      chart.unit,
      evaluation.context.currency,
      locale,
    ),
    comparison: chart.comparison_interval ? point.comparison : undefined,
    comparisonAmount: chart.comparison_interval
      ? format(point.comparison)
      : undefined,
    targetAmount: point.target == null ? undefined : compact(point.target),
    upperAmount: format(point.upper),
  }));
  const onSelect = (key: string) => {
    if (
      chart.kind === "pipeline_movement" &&
      (key === "opening" || key === "closing")
    ) {
      onEvidence({ metric: chart.metric, context_id: `movement_${key}` });
      return;
    }
    const reference = chart.points.find((point) => point.key === key)?.evidence;
    if (reference) onEvidence(reference);
  };
  const shared = {
    readings,
    label: chartTitle(chart, evaluation, t, metricName),
    dataLabel: t("reporting.data"),
    valueLabel: t("reporting.actual"),
    onSelect,
  };
  switch (chart.kind) {
    case "bookings_trend": {
      const target = chart.points.find((point) => point.target != null)?.target;
      return (
        <CumulativeChart
          {...shared}
          comparisonLabel={
            chart.comparison_interval
              ? `${t("reporting.previous")}: ${formatDateTime(chart.comparison_interval.start_at, locale, evaluation.context.timezone)} – ${formatDateTime(new Date(Date.parse(chart.comparison_interval.end_at) - 1).toISOString(), locale, evaluation.context.timezone)}`
              : undefined
          }
          axisLabel={(value) =>
            reportingMoneyUnit(chart.unit, evaluation.context.currency)
              ? formatMoneyCompact(value, evaluation.context.currency, locale)
              : format(value)
          }
          reference={
            target == null
              ? undefined
              : {
                  value: target,
                  amount: compact(target),
                  label: t("reporting.target"),
                }
          }
          size={trendSize}
        />
      );
    }
    case "owner_attainment":
      return <BulletChart {...shared} targetLabel={t("reporting.target")} />;
    case "stage_age":
      return (
        <RangeChart
          {...shared}
          valueLabel={t("reporting.median")}
          upperLabel={t("reporting.upper")}
        />
      );
    case "target_progress":
      return (
        <>
          <BulletChart {...shared} targetLabel={t("reporting.target")} />
          {chart.points.map((point) => (
            <TargetProgress
              key={point.key}
              value={point.value}
              target={point.target}
              format={format}
            />
          ))}
        </>
      );
    case "stage_distribution":
      return (
        <StageDistribution
          chart={chart}
          evaluation={evaluation}
          label={shared.label}
          format={format}
          onSelect={onSelect}
        />
      );
    case "breakdown":
      return (
        <Breakdown
          chart={chart}
          evaluation={evaluation}
          label={shared.label}
          format={format}
          onSelect={onSelect}
        />
      );
    case "stage_cohort":
    case "lead_cohort":
      return (
        <CohortTable
          chart={chart}
          label={shared.label}
          onEvidence={onEvidence}
        />
      );
    case "sdr_outcomes":
      return (
        <SdrOutcomes
          chart={chart}
          evaluation={evaluation}
          onEvidence={onEvidence}
        />
      );
    case "forecast_support": {
      const [won, supported, upside] = readings;
      if (
        !won ||
        !supported ||
        !upside ||
        won.value == null ||
        supported.value == null ||
        upside.value == null
      )
        return <EmptyState>{t("reporting.unavailable")}</EmptyState>;
      return (
        <SegmentBar
          label={shared.label}
          parts={[
            { ...won, value: won.value },
            { ...supported, value: supported.value },
            { ...upside, value: upside.value },
          ]}
          marker={
            chart.marker == null
              ? undefined
              : {
                  key: "manager",
                  label: t("reporting.managerCall"),
                  value: chart.marker,
                  amount: format(chart.marker),
                }
          }
          onSelect={onSelect}
        />
      );
    }
    case "pipeline_movement":
      if (chart.opening == null || chart.closing == null)
        return <EmptyState>{t("reporting.unavailable")}</EmptyState>;
      return (
        <>
          <p className="t-num">
            {t("reporting.netChange", {
              amount: `${chart.closing - chart.opening > 0 ? "+" : ""}${compact(chart.closing - chart.opening)}`,
            })}
          </p>
          <Waterfall
            label={shared.label}
            opening={{
              label: t("reporting.opening"),
              value: chart.opening,
              amount: compact(chart.opening),
            }}
            closing={{
              label: t("reporting.closing"),
              value: chart.closing,
              amount: compact(chart.closing),
            }}
            steps={readings.flatMap((point) =>
              point.value == null
                ? []
                : [
                    {
                      ...point,
                      label: movementLabel(point.key, point.label, t),
                      value: point.value,
                      amount: compact(point.value),
                    },
                  ],
            )}
            reconciliationWarning={t("reporting.reconcile")}
            onSelect={onSelect}
          />
          <Disclosure
            className="report-chart-data"
            summary={t("reporting.data")}
          >
            <DataTable
              label={shared.label}
              rows={[
                {
                  key: "opening",
                  label: t("reporting.opening"),
                  value: chart.opening,
                },
                ...chart.points.map((point) => ({
                  ...point,
                  label: movementLabel(point.key, point.label, t),
                })),
                {
                  key: "closing",
                  label: t("reporting.closing"),
                  value: chart.closing,
                },
              ]}
              rowKey={(point) => point.key}
              columns={[
                {
                  key: "label",
                  header: t("reporting.metrics"),
                  render: (point) => (
                    <Button variant="link" onClick={() => onSelect(point.key)}>
                      {point.label}
                    </Button>
                  ),
                },
                {
                  key: "value",
                  header: t("reporting.actual"),
                  render: (point) => format(point.value),
                },
              ]}
            />
          </Disclosure>
        </>
      );
    case "metric_reading":
      return null;
  }
}

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
