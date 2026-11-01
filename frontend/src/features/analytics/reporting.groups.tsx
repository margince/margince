// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Disclosure } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { BarList } from "../../design-system/readings";
import { DonutChart } from "../../design-system/report-charts";
import { formatNumber, MONEY_ABSENT } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { withDealCount } from "./reporting.dealcount";
import { breakdownLabel } from "./reporting.grouplabel";
import {
  type ReportingChart,
  type ReportingEvaluation,
  reportingCompactAmount,
} from "./reporting.model";
import "./reporting.css";

type ChartPoint = ReportingChart["points"][number];

// The groups a lead or partner figure splits into. A count's records are its
// value, so only a sum names how many records each group holds.
export function Breakdown({
  chart,
  evaluation,
  label,
  format,
  onSelect,
}: Readonly<{
  chart: ReportingChart;
  evaluation: ReportingEvaluation;
  label: string;
  format: (value: number | null | undefined) => string;
  onSelect: (key: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const named = {
    ...chart,
    points: chart.points.map((point) => ({
      ...point,
      label: breakdownLabel(point.key, point.label, t),
    })),
  };
  return (
    <GroupedBars
      chart={named}
      label={label}
      amount={(point) =>
        reportingCompactAmount(
          point.value,
          chart.unit,
          evaluation.context.currency,
          locale,
        )
      }
      format={format}
      countHeader={
        chart.unit === "count" ? undefined : t("reporting.recordCount")
      }
      onSelect={onSelect}
    />
  );
}

// Open pipeline by stage. A few stages draw as a ring round the whole; more
// fall back to the whole over a ranked list.
export function StageDistribution({
  chart,
  evaluation,
  label,
  format,
  onSelect,
}: Readonly<{
  chart: ReportingChart;
  evaluation: ReportingEvaluation;
  label: string;
  format: (value: number | null | undefined) => string;
  onSelect: (key: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { currency } = evaluation.context;
  const headline = reportingCompactAmount(
    evaluation.metrics.find((metric) => metric.id === chart.metric)?.value,
    chart.unit,
    currency,
    locale,
  );
  const ring = asWhole(chart);
  return (
    <>
      {!ring && <p className="reporting-headline t-num">{headline}</p>}
      <GroupedBars
        chart={chart}
        label={label}
        whole={
          ring
            ? { total: headline, label: t("reporting.selectedValue") }
            : undefined
        }
        amount={(point) =>
          withDealCount(
            reportingCompactAmount(point.value, chart.unit, currency, locale),
            point.observations,
            plural,
            locale,
          )
        }
        format={format}
        countHeader={t("reporting.stageDeals")}
        onSelect={onSelect}
      />
    </>
  );
}

// A figure split into groups: a bar per group that opens its records, and the
// same groups as a table under a disclosure.
function GroupedBars({
  chart,
  label,
  amount,
  format,
  countHeader,
  onSelect,
  whole,
}: Readonly<{
  chart: ReportingChart;
  label: string;
  amount: (point: ChartPoint & { value: number }) => string;
  format: (value: number | null | undefined) => string;
  countHeader?: string;
  onSelect: (key: string) => void;
  // The groups are parts of this whole, so they draw as a ring around it.
  whole?: Readonly<{ total: string; label: string }>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = chart.points.flatMap((point) =>
    point.value == null
      ? []
      : [
          {
            key: point.key,
            label: point.label,
            value: point.value,
            amount: amount({ ...point, value: point.value }),
          },
        ],
  );
  return (
    <>
      {whole ? (
        <DonutChart
          label={label}
          parts={rows}
          total={whole.total}
          totalLabel={whole.label}
          onSelect={onSelect}
        />
      ) : (
        <BarList label={label} rows={rows} onSelect={onSelect} />
      )}
      <Disclosure className="report-chart-data" summary={t("reporting.data")}>
        <DataTable
          label={label}
          rows={chart.points}
          rowKey={(point) => point.key}
          columns={[
            {
              key: "label",
              header: label,
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
            ...(countHeader == null
              ? []
              : [
                  {
                    key: "count",
                    header: countHeader,
                    render: (point: ChartPoint) =>
                      point.observations == null
                        ? MONEY_ABSENT
                        : formatNumber(point.observations, locale),
                  },
                ]),
          ]}
        />
      </Disclosure>
    </>
  );
}

// Every part read, few enough to tell apart round a ring, none below zero,
// and something to divide: past that a ranked list reads better.
const RING_PARTS = 6;
function asWhole(chart: ReportingChart): boolean {
  const { points } = chart;
  return (
    points.length > 1 &&
    points.length <= RING_PARTS &&
    points.every((point) => point.value != null && point.value >= 0) &&
    points.some((point) => (point.value ?? 0) > 0)
  );
}
