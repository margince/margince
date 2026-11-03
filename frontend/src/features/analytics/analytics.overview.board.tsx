// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { ChartPanel } from "./reporting.charts";
import type {
  ReportingBlockKind,
  ReportingChart,
  ReportingEvaluation,
  ReportingEvidenceRef,
  ReportingSelection,
} from "./reporting.model";
import { useReportingEvaluation } from "./reporting.queries";

// The sales charts the opening page draws. Stage age stays on Performance: it
// answers how long deals wait, which a review opens only when a stage stalls.
const SALES_BOARD_BLOCKS: ReadonlySet<ReportingBlockKind> = new Set([
  "bookings_trend",
  "stage_distribution",
  "owner_attainment",
  "breakdown",
]);

/** The charts of the sales preset that the opening page asks for. */
export function salesBoardBlocks(
  preset: readonly ReportingBlockKind[],
): ReportingBlockKind[] {
  return preset.filter((kind) => SALES_BOARD_BLOCKS.has(kind));
}

/**
 * The chart read: the board's charts and only the metrics they belong to. The
 * figures answer without waiting for a chart, and no figure-only metric is
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
          <BoardChart
            evaluation={read}
            kind="breakdown"
            onEvidence={(reference) => onEvidence(reference, read)}
          />
        </div>
      )}
    </QueryGate>
  );
}
