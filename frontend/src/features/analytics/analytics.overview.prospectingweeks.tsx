// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { useProspectingWeeks } from "./analytics.overview.prospectingweeks.queries";

export {
  outcomeWeeks,
  type ProspectingOutcome,
  useProspectingWeeks,
} from "./analytics.overview.prospectingweeks.queries";

import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { GroupedBars } from "../../design-system/report-charts";
import { formatDateAbbrev, formatDayMonth } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import {
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";
import { SdrOutcomes } from "./reporting.sdroutcomes";

// A week is named by its Monday. A bucket with no value stays empty: it is
// never drawn as a measured zero.
function weekReadings(
  chart: ReportingChart,
  evaluation: ReportingEvaluation,
  locale: Locale,
) {
  const { timezone } = evaluation.context;
  return chart.points.map((point) => ({
    ...point,
    label: formatDayMonth(point.at ?? point.key, locale, timezone),
    amount: reportingAmount(point.value, chart.unit, "", locale),
  }));
}

/** The outcome's recent weeks as short columns, beside the lead figure. */
export function OutcomeWeeks({
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
  const metricName = useMetricNames();
  return (
    <GroupedBars
      size="compact"
      label={t("analytics.prospect.weeksLabel", {
        metric: metricName(chart.metric),
      })}
      valueLabel={metricName(chart.metric)}
      dataLabel={t("reporting.data")}
      readings={weekReadings(chart, evaluation, locale)}
      onSelect={(key) => {
        const reference = chart.points.find(
          (point) => point.key === key,
        )?.evidence;
        if (reference) onEvidence(reference);
      }}
    />
  );
}

/**
 * How the outcomes developed over the recent weeks: held meetings beside the
 * outcome, each in its own week, with the weeks named.
 */
export function OutcomeHistory({
  weeks,
  onEvidence,
}: Readonly<{
  weeks: ReturnType<typeof useProspectingWeeks>;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Panel title={t("analytics.prospect.historyTitle")} flatHead>
      <PanelBody>
        <QueryGate
          query={weeks}
          pendingLabel={t("analytics.prospect.historyTitle")}
        >
          {(evaluation) => (
            <>
              <p className="t-caption">
                {t("analytics.prospect.historyCaption", {
                  from: formatDateAbbrev(
                    evaluation.context.interval.start_at,
                    locale,
                    evaluation.context.timezone,
                  ),
                })}{" "}
                {t(
                  evaluation.charts.some(
                    (chart) => chart.metric === "leads_qualified",
                  )
                    ? "analytics.prospect.weeksByQualification"
                    : evaluation.charts.some(
                          (chart) => chart.metric === "leads_promoted",
                        )
                      ? "analytics.prospect.weeksByArrival"
                      : "analytics.prospect.weeksByAcceptance",
                )}
                {underWay(evaluation) &&
                  ` ${t("analytics.prospect.weeksUnderWay")}`}
              </p>
              {evaluation.charts
                .filter(
                  (chart) =>
                    chart.kind === "sdr_outcomes" &&
                    chart.metric !== "accepted_opportunities",
                )
                .map((chart) => (
                  <SdrOutcomes
                    key={chart.metric}
                    chart={{
                      ...chart,
                      points: weekReadings(chart, evaluation, locale),
                    }}
                    evaluation={evaluation}
                    onEvidence={onEvidence}
                  />
                ))}
            </>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

// Whether the weeks reach past the moment they were read, so the last one is
// still under way.
function underWay(evaluation: ReportingEvaluation): boolean {
  const { interval, evaluated_at } = evaluation.context;
  return Date.parse(interval.end_at) > Date.parse(evaluated_at);
}
