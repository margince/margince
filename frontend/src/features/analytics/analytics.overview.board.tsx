// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";
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

/** The evaluation's chart of this kind, unless it has nothing to draw. */
export function drawnChart(
  evaluation: ReportingEvaluation,
  kind: ReportingChart["kind"],
): ReportingChart | undefined {
  const chart = evaluation.charts.find((candidate) => candidate.kind === kind);
  if (
    !chart ||
    (chart.coverage.status === "no_data" &&
      chart.points.every((point) => point.value == null || point.value === 0))
  )
    return undefined;
  return chart;
}

/** One preset chart in its panel, or nothing when the evaluation lacks it. */
export function BoardChart({
  evaluation,
  kind,
  onEvidence,
  section = false,
  meta,
  note,
}: Readonly<{
  evaluation: ReportingEvaluation;
  kind: ReportingChart["kind"];
  onEvidence: (reference: ReportingEvidenceRef) => void;
  // A section of a sheet, with a flat head, rather than a panel of its own.
  section?: boolean;
  meta?: ReactNode;
  note?: ReactNode;
}>) {
  const chart = drawnChart(evaluation, kind);
  if (!chart) return null;
  return (
    <ChartPanel
      chart={chart}
      wide={false}
      section={section}
      meta={meta}
      note={note}
      evaluation={evaluation}
      onEvidence={onEvidence}
    />
  );
}

/**
 * Today's stock by stage over the permitted sales by owner, from the chart
 * read, each a section of the review's sheet. While it is read, or when it
 * failed, it says so in its own place and the figures above it stand.
 */
export function StockCharts({
  charts,
  stageMeta,
  stageNote,
  onEvidence,
}: Readonly<{
  charts: UseQueryResult<ReportingEvaluation>;
  // Said in the stage chart's head, and above its bars.
  stageMeta?: ReactNode;
  stageNote?: ReactNode;
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
}>) {
  const t = useT();
  return (
    <QueryGate query={charts} pendingLabel={t("analytics.overview.charts")}>
      {(read) => (
        <>
          <BoardChart
            evaluation={read}
            kind="stage_distribution"
            onEvidence={(reference) => onEvidence(reference, read)}
            section
            meta={stageMeta}
            note={stageNote}
          />
          <BoardChart
            evaluation={read}
            kind="owner_attainment"
            onEvidence={(reference) => onEvidence(reference, read)}
            section
          />
          <BoardChart
            evaluation={read}
            kind="breakdown"
            onEvidence={(reference) => onEvidence(reference, read)}
            section
          />
        </>
      )}
    </QueryGate>
  );
}
