// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { useLocale, useT } from "../../i18n";
import { metricOf, readingValue } from "./analytics.overview.readings";
import { ChartBody } from "./reporting.chartbody";
import { ChartContext, ChartDetails } from "./reporting.chartcontext";
import { ChartPanel } from "./reporting.charts";
import {
  type ReportingBlockKind,
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingPeriodLabel,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";

// The sales charts the opening page draws. Stage age stays on Performance: it
// answers how long deals wait, which a review opens only when a stage stalls.
const SALES_BOARD_BLOCKS: ReadonlySet<ReportingBlockKind> = new Set([
  "bookings_trend",
  "stage_distribution",
  "owner_attainment",
]);

/** The charts of the sales preset that the opening page asks for. */
export function salesBoardBlocks(
  preset: readonly ReportingBlockKind[],
): ReportingBlockKind[] {
  return preset.filter((kind) => SALES_BOARD_BLOCKS.has(kind));
}

function chartOf(
  evaluation: ReportingEvaluation,
  kind: ReportingChart["kind"],
): ReportingChart | undefined {
  return evaluation.charts.find((chart) => chart.kind === kind);
}

const drawn = (chart: ReportingChart | undefined): chart is ReportingChart =>
  chart !== undefined &&
  chart.points.length > 0 &&
  chart.coverage.status !== "no_data";

/**
 * Sales won as the page's lead: the figure, the period it covers and the door
 * to its deals, beside the running total that reached it.
 */
export function SalesHero({
  evaluation,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const metricName = useMetricNames();
  const { context } = evaluation;
  const won = metricOf(evaluation, "bookings_won");
  const trend = chartOf(evaluation, "bookings_trend");
  const opens =
    won?.value != null &&
    won.coverage.status !== "no_data" &&
    won.coverage.status !== "failed";
  return (
    <Panel
      title={metricName("bookings_won")}
      titleAction={
        trend && <ChartDetails chart={trend} evaluation={evaluation} />
      }
      footer={trend && <ChartContext chart={trend} evaluation={evaluation} />}
    >
      <PanelBody>
        <div className="analytics-hero-body">
          <div className="analytics-hero-figure">
            <p className="analytics-hero-value t-num">
              {readingValue(won, evaluation, t, locale)}
            </p>
            {/* With the chart drawn, its foot names the period. */}
            {!trend && (
              <p className="t-caption">
                {reportingPeriodLabel(
                  context.interval,
                  context.timezone,
                  locale,
                )}
              </p>
            )}
            {won?.coverage.withheld && (
              <p className="t-caption">
                {t("analytics.overview.partlyHidden")}
              </p>
            )}
            {won?.coverage.status !== "ok" && won?.coverage.reason && (
              <p className="t-caption">{won.coverage.reason}</p>
            )}
            {opens && (
              <Button variant="link" onClick={() => onEvidence(won.evidence)}>
                {t("reporting.viewRecords")}
              </Button>
            )}
          </div>
          {drawn(trend) && (
            <div className="analytics-hero-chart">
              <ChartBody
                chart={trend}
                evaluation={evaluation}
                onEvidence={onEvidence}
              />
            </div>
          )}
        </div>
      </PanelBody>
    </Panel>
  );
}

/** One preset chart in its panel, or nothing when the evaluation lacks it. */
export function BoardChart({
  evaluation,
  kind,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  kind: ReportingChart["kind"];
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const chart = chartOf(evaluation, kind);
  if (!chart) return null;
  return (
    <ChartPanel
      chart={chart}
      wide={false}
      evaluation={evaluation}
      onEvidence={onEvidence}
    />
  );
}
