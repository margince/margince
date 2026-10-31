// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { GroupedBars } from "../../design-system/report-charts";
import { formatDateAbbrev, formatDayMonth } from "../../format/format";
import { dayInZone, startOfDayInZone } from "../../format/timezone";
import { type Locale, useLocale, useT } from "../../i18n";
import {
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingMetricID,
  type ReportingSelection,
  reportingAmount,
} from "./reporting.model";
import { useMetricNames, useReportingEvaluation } from "./reporting.queries";
import { SdrOutcomes } from "./reporting.sdroutcomes";

// How many weeks the trend shows, the one under way included.
const WEEKS = 6;

export type ProspectingOutcome = Extract<
  ReportingMetricID,
  "accepted_opportunities" | "leads_promoted"
>;

/**
 * The weeks that end with the period on screen, read as the server's own
 * weekly buckets. The interval is cut from the answered period in its zone.
 * So when the period is a week, the last bucket is the headline's week.
 */
export function useProspectingWeeks(
  week: ReportingEvaluation | undefined,
  outcome: ProspectingOutcome | undefined,
) {
  const selection: ReportingSelection | undefined =
    week && outcome
      ? {
          scope: week.selection.scope,
          period: "custom",
          interval: weeksBefore(week),
          target_basis: "month",
          close_window: "all_open",
          metrics: [outcome, "meetings_held"],
          blocks: ["sdr_outcomes"],
        }
      : undefined;
  // A disabled query still needs a key, so it asks for nothing it would send.
  return useReportingEvaluation(
    selection ?? NO_SELECTION,
    selection !== undefined,
  );
}

const NO_SELECTION: ReportingSelection = {
  scope: { kind: "owner" },
  period: "this_week",
  target_basis: "month",
  close_window: "all_open",
  metrics: [],
  blocks: [],
};

function weeksBefore(
  week: ReportingEvaluation,
): ReportingSelection["interval"] {
  const { interval, timezone } = week.context;
  const last = dayInZone(Date.parse(interval.end_at) - 1, timezone);
  const weekday = new Date(`${last}T00:00:00Z`).getUTCDay();
  const fromMonday = (weekday + 6) % 7;
  // Whole weeks: the server stops the week under way at its own reading.
  return {
    start_at: startOfDayInZone(last, timezone, -fromMonday - 7 * (WEEKS - 1)),
    end_at: startOfDayInZone(last, timezone, 7 - fromMonday),
  };
}

/** The outcome's weekly buckets, or undefined while there are none to draw. */
export function outcomeWeeks(
  weeks: ReportingEvaluation | undefined,
  outcome: ProspectingOutcome,
): ReportingChart | undefined {
  const chart = weeks?.charts.find(
    (each) => each.kind === "sdr_outcomes" && each.metric === outcome,
  );
  return chart && chart.points.length > 0 ? chart : undefined;
}

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
                    (chart) => chart.metric === "leads_promoted",
                  )
                    ? "analytics.prospect.weeksByArrival"
                    : "analytics.prospect.weeksByAcceptance",
                )}
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
