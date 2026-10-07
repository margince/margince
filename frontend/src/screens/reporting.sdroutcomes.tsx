import { Popover } from "../design-system/popover";
import { GroupedBars } from "../design-system/report-charts";
import { useLocale, useT } from "../i18n";
import {
  metricLabel,
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
} from "./reporting.model";

export function SdrOutcomes({
  chart,
  evaluation,
  onEvidence,
}: Readonly<{
  chart: ReportingChart;
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const format = (value: number | null | undefined) =>
    reportingAmount(value, chart.unit, evaluation.context.currency, locale);
  const readings = chart.points.map((point) => ({
    ...point,
    amount: format(point.value),
  }));
  const onSelect = (key: string) => {
    const ref = chart.points.find((point) => point.key === key)?.evidence;
    if (ref) onEvidence(ref);
  };
  const shared = {
    readings,
    label: metricLabel(chart.metric, t),
    dataLabel: t("reporting.data"),
    valueLabel: t("reporting.actual"),
    onSelect,
  };

  const accepted =
    chart.metric === "meetings_held"
      ? evaluation.charts.find(
          (other) =>
            other.kind === "sdr_outcomes" &&
            other.metric === "accepted_opportunities",
        )
      : undefined;
  const combined = readings.map((reading) => {
    const comparison = accepted?.points.find(
      (point) => point.key === reading.key,
    )?.value;
    return { ...reading, comparison, comparisonAmount: format(comparison) };
  });
  return (
    <>
      {accepted && (
        <Popover onHover label={t("reporting.definition")}>
          <p>{t("reporting.independent")}</p>
        </Popover>
      )}
      <GroupedBars
        {...shared}
        readings={combined}
        valueLabel={metricLabel(chart.metric, t)}
        comparisonLabel={
          accepted ? t("reporting.accepted_opportunities") : undefined
        }
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
