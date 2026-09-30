import { Button, Disclosure, EmptyState } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { BarList, SegmentBar } from "../design-system/readings";
import {
  BulletChart,
  CumulativeChart,
  GroupedBars,
  RangeChart,
} from "../design-system/report-charts";
import { Waterfall } from "../design-system/waterfall";
import { formatDateTime, formatMoneyCompact } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  blockLabel,
  metricLabel,
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingMoneyUnit,
} from "./reporting.model";
import { ReportingScorecard } from "./reporting.scorecard";
import { worklistLaneHref } from "./worklist.header";
import "./reporting.css";

type EvidenceAction = (reference: ReportingEvidenceRef) => void;

export function ReportingCharts({
  evaluation,
  editionId,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  editionId?: string;
  onEvidence: EvidenceAction;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const amount = (value: number | null | undefined, unit: string) =>
    reportingAmount(value, unit, evaluation.context.currency, locale);
  const charts = evaluation.charts.filter(
    (chart) =>
      chart.kind !== "metric_reading" &&
      !(
        chart.kind === "sdr_outcomes" &&
        chart.metric === "accepted_opportunities"
      ),
  );
  return (
    <>
      <div className="reporting-grid">
        {charts.map((chart) => (
          <Panel
            key={`${chart.kind}:${chart.metric}`}
            className={`reporting-panel reporting-panel-${chart.kind}`}
            title={blockLabel(chart.kind, t)}
          >
            <PanelBody>
              <ChartContext
                chart={chart}
                evaluation={evaluation}
                editionId={editionId}
              />
              {chart.points.length ? (
                <ChartBody
                  chart={chart}
                  evaluation={evaluation}
                  onEvidence={onEvidence}
                />
              ) : (
                <EmptyState>
                  {chart.coverage.reason ?? t("common.empty")}
                </EmptyState>
              )}
            </PanelBody>
          </Panel>
        ))}
      </div>
      <ReportingScorecard
        evaluation={evaluation}
        editionId={editionId}
        onEvidence={onEvidence}
      />
      <Disclosure summary={t("reporting.details")}>
        <DataTable
          label={t("reporting.metrics")}
          rows={evaluation.metrics}
          rowKey={(metric) => metric.id}
          columns={[
            {
              key: "metric",
              header: t("reporting.metrics"),
              render: (metric) => (
                <Button
                  variant="link"
                  onClick={() => onEvidence(metric.evidence)}
                >
                  {metricLabel(metric.id, t)}
                </Button>
              ),
            },
            {
              key: "actual",
              header: t("reporting.actual"),
              render: (metric) => amount(metric.value, metric.unit),
            },
            {
              key: "target",
              header: t("reporting.target"),
              render: (metric) =>
                metric.target == null
                  ? t("reporting.noTarget")
                  : amount(metric.target, metric.unit),
            },
            {
              key: "status",
              header: t("reporting.evidence"),
              render: (metric) =>
                metric.coverage.reason ?? metric.coverage.status,
            },
          ]}
        />
      </Disclosure>
    </>
  );
}

function ChartBody({
  chart,
  evaluation,
  onEvidence,
}: Readonly<{
  chart: ReportingChart;
  evaluation: ReportingEvaluation;
  onEvidence: EvidenceAction;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const format = (value: number | null | undefined) =>
    reportingAmount(value, chart.unit, evaluation.context.currency, locale);
  const compact = (value: number) =>
    reportingMoneyUnit(chart.unit, evaluation.context.currency)
      ? formatMoneyCompact(value, evaluation.context.currency, locale)
      : format(value);
  const readings = chart.points.map((point) => ({
    ...point,
    amount: format(point.value),
    comparisonAmount: format(point.comparison),
    targetAmount:
      point.target == null ? t("reporting.noTarget") : format(point.target),
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
    label: blockLabel(chart.kind, t),
    dataLabel: t("reporting.data"),
    valueLabel: t("reporting.actual"),
    onSelect,
  };
  switch (chart.kind) {
    case "bookings_trend": {
      const target = chart.points.find((point) => point.target != null)?.target;
      const metric = evaluation.metrics.find(
        (metric) => metric.id === chart.metric,
      );
      return (
        <>
          <p className="reporting-headline t-num">{format(metric?.value)}</p>
          <CumulativeChart
            {...shared}
            comparisonLabel={t("reporting.previous")}
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
                    amount: format(target),
                    label: t("reporting.target"),
                  }
            }
          />
        </>
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
    case "stage_distribution":
    case "target_progress":
      return (
        <>
          {chart.kind === "stage_distribution" && (
            <p className="reporting-headline t-num">
              {format(
                evaluation.metrics.find((metric) => metric.id === chart.metric)
                  ?.value,
              )}
            </p>
          )}
          <BarList
            label={shared.label}
            rows={readings.flatMap((point) =>
              point.value == null
                ? []
                : [
                    {
                      key: point.key,
                      label: point.label,
                      value: point.value,
                      amount: reportingMoneyUnit(
                        chart.unit,
                        evaluation.context.currency,
                      )
                        ? formatMoneyCompact(
                            point.value,
                            evaluation.context.currency,
                            locale,
                          )
                        : point.amount,
                    },
                  ],
            )}
            onSelect={onSelect}
          />
          <Disclosure summary={t("reporting.data")}>
            <DataTable
              label={shared.label}
              rows={chart.points}
              rowKey={(point) => point.key}
              columns={[
                {
                  key: "label",
                  header: shared.label,
                  render: (point) => point.label,
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
    case "sdr_outcomes": {
      const accepted = evaluation.charts.find(
        (other) =>
          other.kind === "sdr_outcomes" &&
          other.metric === "accepted_opportunities",
      );
      const combined = readings.map((reading) => {
        const comparison = accepted?.points.find(
          (point) => point.key === reading.key,
        )?.value;
        return { ...reading, comparison, comparisonAmount: format(comparison) };
      });
      return (
        <>
          <p className="t-caption">{t("reporting.independent")}</p>
          <GroupedBars
            {...shared}
            readings={combined}
            valueLabel={t("reporting.meetings_held")}
            comparisonLabel={t("reporting.accepted_opportunities")}
            onSelect={(key) => {
              if (key.endsWith(":comparison")) {
                const reference = accepted?.points.find(
                  (point) => `${point.key}:comparison` === key,
                )?.evidence;
                if (reference) onEvidence(reference);
              } else onSelect(key);
            }}
          />
        </>
      );
    }
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
                      value: point.value,
                      amount: compact(point.value),
                    },
                  ],
            )}
            reconciliationWarning={t("reporting.reconcile")}
            onSelect={onSelect}
          />
          <Disclosure summary={t("reporting.data")}>
            <DataTable
              label={shared.label}
              rows={[
                {
                  key: "opening",
                  label: t("reporting.opening"),
                  value: chart.opening,
                },
                ...chart.points,
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
                  render: (point) => point.label,
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

function ChartContext({
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
                  chart.interval.end_at,
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
        <p className="t-caption" role="status">
          {chart.coverage.reason ??
            t("reporting.coverage", { status: chart.coverage.status })}
        </p>
      )}
    </>
  );
}
