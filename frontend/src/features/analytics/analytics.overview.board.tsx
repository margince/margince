// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useLocale, useT } from "../../i18n";
import { metricOf, readingValue } from "./analytics.overview.readings";
import { ChartBody } from "./reporting.chartbody";
import { ChartDetails } from "./reporting.chartcontext";
import { ChartPanel } from "./reporting.charts";
import {
  type ReportingBlockKind,
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingPeriodLabel,
} from "./reporting.model";
import { useMetricNames, useReportingEvaluation } from "./reporting.queries";

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

/**
 * The chart read: the board's charts and only the metrics they belong to, so
 * the figures answer without waiting for a chart, and no figure-only metric is
 * evaluated twice. A preset with no board chart asks nothing.
 */
export function useSalesCharts(
  headline: ReportingSelection,
  preset: readonly ReportingBlockKind[],
  catalog: Readonly<{
    metrics: readonly Readonly<{
      id: string;
      blocks: readonly ReportingBlockKind[];
    }>[];
  }>,
  enabled: boolean,
): UseQueryResult<ReportingEvaluation> | undefined {
  const blocks = salesBoardBlocks(preset);
  // Each chart with the first asked metric that owns it, in the preset's order.
  const owners = blocks.map((block) =>
    headline.metrics.find((id) =>
      catalog.metrics.some(
        (metric) => metric.id === id && metric.blocks.includes(block),
      ),
    ),
  );
  const metrics = headline.metrics.filter((id) => owners.includes(id));
  const asked = blocks.length > 0 && metrics.length > 0;
  const query = useReportingEvaluation(
    { ...headline, metrics, blocks },
    enabled && asked,
  );
  return asked ? query : undefined;
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

// A running total that never leaves zero, in either period, is a blank box:
// the figure and its explanation say it better alone.
const moves = (chart: ReportingChart): boolean =>
  chart.points.some((point) => !!point.value || !!point.comparison);

/**
 * Sales won as the page's lead: the figure, the period it covers and the door
 * to its deals, beside the running total that reached it. `aside` stands
 * under the figure: the target it is read against, and the period before.
 */
export function SalesHero({
  evaluation,
  charts,
  onEvidence,
  aside,
}: Readonly<{
  evaluation: ReportingEvaluation;
  // The chart read, apart from the figures: pending, failed or answered, or
  // none when the preset draws no chart.
  charts?: Readonly<{ data?: ReportingEvaluation; isPending: boolean }>;
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
  aside?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const metricName = useMetricNames();
  const { context } = evaluation;
  const won = metricOf(evaluation, "bookings_won");
  const found = charts?.data && chartOf(charts.data, "bookings_trend");
  const trend = drawn(found) && moves(found) ? found : undefined;
  const trendRead = charts?.data;
  return (
    <Panel
      title={metricName("bookings_won")}
      // One line of head and no foot: the header's caption already names the
      // period, and the first screen has no height to spare.
      flatHead
      titleAction={
        trend &&
        trendRead && <ChartDetails chart={trend} evaluation={trendRead} />
      }
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
            {won && (
              <Button variant="link" onClick={() => onEvidence(won.evidence)}>
                {t("reporting.viewRecords")}
              </Button>
            )}
            {aside}
          </div>
          {trend && trendRead && (
            <div className="analytics-hero-chart">
              <ChartBody
                chart={trend}
                evaluation={trendRead}
                onEvidence={(reference) => onEvidence(reference, trendRead)}
                trendSize="compact"
              />
            </div>
          )}
          {/* The trend's place, kept while it is read, so the figure does not
              move when it lands. */}
          {charts?.isPending && (
            <div className="analytics-hero-chart" aria-hidden="true" />
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

/**
 * Today's stock by stage beside the permitted sales by owner, from the chart
 * read. While it is read, or when it failed, it says so in its own place and
 * the figures above it stand.
 */
export function StockCharts({
  charts,
  onEvidence,
}: Readonly<{
  charts: UseQueryResult<ReportingEvaluation>;
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
}>) {
  const t = useT();
  return (
    <QueryGate query={charts} pendingLabel={t("analytics.overview.charts")}>
      {(read) => (
        <div className="analytics-pair">
          <BoardChart
            evaluation={read}
            kind="stage_distribution"
            onEvidence={(reference) => onEvidence(reference, read)}
          />
          <BoardChart
            evaluation={read}
            kind="owner_attainment"
            onEvidence={(reference) => onEvidence(reference, read)}
          />
        </div>
      )}
    </QueryGate>
  );
}
