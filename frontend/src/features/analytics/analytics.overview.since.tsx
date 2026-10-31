// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EmptyState } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import type { AttentionState } from "./analytics.attention";
import { ChartContext } from "./reporting.chartcontext";
import { ChartPanel } from "./reporting.charts";
import {
  ReportingEvidenceDrawer,
  useSelectionEvidence,
} from "./reporting.evidence";
import type {
  ReportingEvaluation,
  ReportingSelection,
} from "./reporting.model";
import { useReportingEvaluation } from "./reporting.queries";

type Catalog = components["schemas"]["ReportingCatalog"];

// How the open pipeline moved between the two latest compatible captures. It
// is read once the action list has its answer, so at most two optional reads
// are in flight. With fewer than two captures it says when comparison starts.
export function SinceLastReview({
  catalog,
  scope,
  pipelineId,
  after,
}: Readonly<{
  catalog: Catalog;
  scope: ReportingSelection["scope"];
  pipelineId: string | undefined;
  after: AttentionState;
}>) {
  const t = useT();
  const offered = catalog.metrics.some(
    (metric) =>
      metric.id === "open_pipeline" &&
      metric.blocks.includes("pipeline_movement"),
  );
  const selection: ReportingSelection = {
    scope,
    pipeline_id: pipelineId,
    period: "this_quarter",
    target_basis: "fiscal_quarter",
    // Captures hold the deals expected to close this quarter.
    close_window: "fiscal_quarter",
    metrics: ["open_pipeline"],
    blocks: ["pipeline_movement"],
  };
  const query = useReportingEvaluation(
    selection,
    offered && after !== "pending",
  );
  const { evidence, openEvidence, closeEvidence } = useSelectionEvidence(
    JSON.stringify(selection),
  );
  if (!offered) return null;
  return (
    <QueryGate query={query} pendingLabel={t("analytics.overview.sinceTitle")}>
      {(evaluation) => {
        const chart = evaluation.charts.find(
          (each) => each.kind === "pipeline_movement",
        );
        if (!chart) return null;
        if (chart.opening_snapshot_id && chart.points.length > 0) {
          return (
            <>
              <ChartPanel
                chart={chart}
                wide
                evaluation={evaluation}
                onEvidence={openEvidence}
              />
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={closeEvidence}
                />
              )}
            </>
          );
        }
        return (
          <Panel
            title={t("analytics.overview.sinceTitle")}
            footer={<ChartContext chart={chart} evaluation={evaluation} />}
          >
            <PanelBody>
              <EmptyState>{movementState(chart, t)}</EmptyState>
            </PanelBody>
          </Panel>
        );
      }}
    </QueryGate>
  );
}

// Why the movement has nothing to draw: the reader cannot have it, the two
// captures agree, or there is no second capture yet.
function movementState(
  chart: ReportingEvaluation["charts"][number],
  t: ReturnType<typeof useT>,
): string {
  const { status, reason } = chart.coverage;
  if (status !== "ok" && status !== "no_data")
    return reason ?? t(`reporting.status.${status}`);
  return chart.opening_snapshot_id
    ? t("analytics.overview.noMovement")
    : t("analytics.overview.historyStarts");
}
