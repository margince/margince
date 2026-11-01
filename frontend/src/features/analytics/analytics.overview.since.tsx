// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { QueryGate } from "../../design-system/querystates";
import { formatDateAbbrev } from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { AttentionState } from "./analytics.attention";
import { ChartPanel } from "./reporting.charts";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { useSelectionEvidence } from "./reporting.evidenceselection";
import type {
  ReportingEvaluation,
  ReportingSelection,
} from "./reporting.model";
import { useReportingEvaluation } from "./reporting.queries";

type Catalog = components["schemas"]["ReportingCatalog"];

// How the open pipeline moved between the two latest compatible captures. It
// is read once the action list has its answer, so at most two optional reads
// are in flight. With fewer than two captures it says, in one line, when
// history started and when the movement shows.
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
  const { locale } = useLocale();
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
  const { evidence, evidenceFrom, openEvidence, closeEvidence } =
    useSelectionEvidence(JSON.stringify(selection), query.data);
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
                  evaluation={evidenceFrom ?? evaluation}
                  reference={evidence}
                  onClose={closeEvidence}
                />
              )}
            </>
          );
        }
        return (
          <p className="analytics-open-line">
            <span className="analytics-attention-title">
              {t("analytics.overview.sinceTitle")}:
            </span>{" "}
            <span className="t-caption">
              {movementState(chart, evaluation.context.timezone, t, locale)}
            </span>
          </p>
        );
      }}
    </QueryGate>
  );
}

// Why the movement has nothing to draw, in one line rather than an empty panel.
// The reader cannot have it, or the two captures agree, or history is still
// starting; then the line dates the captures it waits for.
function movementState(
  chart: ReportingEvaluation["charts"][number],
  zone: string,
  t: Translator,
  locale: Locale,
): string {
  const { status, reason } = chart.coverage;
  if (chart.opening_snapshot_id) return t("analytics.overview.noMovement");
  const next = chart.capture_status?.next_capture_at;
  const unsupported = status !== "ok" && status !== "no_data";
  if (!next || (unsupported && status !== "unavailable"))
    return unsupported
      ? (reason ?? t(`reporting.status.${status}`))
      : t("analytics.overview.historyStarts");
  const day = (at: string) => formatDateAbbrev(at, locale, zone);
  return chart.state_at
    ? t("analytics.overview.historyFrom", {
        date: day(chart.state_at),
        next: day(next),
      })
    : t("analytics.overview.historyNext", { next: day(next) });
}
