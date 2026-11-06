// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { ErrorLine } from "../../design-system/errorline";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { drawnChart, StockCharts } from "./analytics.overview.board";
import { HeroAside, heroReceipt } from "./analytics.overview.hero";
import { SalesOverviewMetrics } from "./analytics.overview.metrics";
import {
  OpenPipelineLine,
  OpenPipelineMeta,
  OpenPipelineNote,
} from "./analytics.overview.salesreadings";
import { QuarterTrajectory } from "./analytics.overview.trajectory";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
  ReportingSelection,
} from "./reporting.model";

// The review's figures on its sheet: the headline readings with the
// trajectory, then the stock. An address the server refused says why instead.
export function SalesFigures({
  query,
  invalid,
  forecastHref,
  charts,
  onApply,
  onEvidence,
  targetSetup,
}: Readonly<{
  query: UseQueryResult<ReportingEvaluation>;
  invalid: boolean;
  forecastHref: string;
  charts: UseQueryResult<ReportingEvaluation> | undefined;
  onApply: (selection: ReportingSelection) => void;
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
  targetSetup: ReactNode;
}>) {
  const t = useT();
  if (invalid) return <ErrorLine error={query.error} />;
  const chartsRead = charts?.isError ? undefined : charts?.data;
  return (
    <QueryGate query={query} pendingLabel={t("analytics.overview.sales")}>
      {(evaluation) => (
        <>
          <SalesOverviewMetrics
            evaluation={evaluation}
            forecastHref={forecastHref}
            onEvidence={onEvidence}
            aside={
              <HeroAside
                evaluation={evaluation}
                onApply={onApply}
                targetSetup={targetSetup}
              />
            }
            receipt={heroReceipt(evaluation, chartsRead)}
            trajectory={
              <QuarterTrajectory
                evaluation={evaluation}
                charts={chartsRead}
                onEvidence={onEvidence}
              />
            }
          />
          {/* Today's stock by stage, and the permitted chart of sales by
              owner, in name order. */}
          <SalesStock
            answered={evaluation}
            charts={charts}
            onEvidence={onEvidence}
          />
        </>
      )}
    </QueryGate>
  );
}

// Today's stock by stage and the permitted sales by owner, each a section of
// the review's sheet. All open pipeline heads the stage chart, which draws the
// same stock; with no stage chart to head, it keeps a line of its own.
function SalesStock({
  answered,
  charts,
  onEvidence,
}: Readonly<{
  answered: ReportingEvaluation;
  charts: UseQueryResult<ReportingEvaluation> | undefined;
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
}>) {
  const stage = charts?.data
    ? drawnChart(charts.data, "stage_distribution")
    : undefined;
  const headed =
    charts?.isError !== true &&
    (charts?.data ? stage != null : charts?.isPending === true);
  return (
    <>
      {!headed && (
        <OpenPipelineLine evaluation={answered} onEvidence={onEvidence} />
      )}
      {charts && (
        <StockCharts
          charts={charts}
          stageMeta={
            headed ? <OpenPipelineMeta evaluation={answered} /> : undefined
          }
          stageNote={
            headed ? (
              <OpenPipelineNote
                evaluation={answered}
                chartReason={
                  stage?.coverage.withheld ? stage.coverage.reason : undefined
                }
              />
            ) : undefined
          }
          onEvidence={onEvidence}
        />
      )}
    </>
  );
}
