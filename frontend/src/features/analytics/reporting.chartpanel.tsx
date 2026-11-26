import { type ReactNode, Suspense, useState } from "react";
import { Button, EmptyState, PendingBody } from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { Panel, PanelBody } from "../../design-system/panel";
import { ChartTableSlotProvider } from "../../design-system/report-charts.tabledoor";
import { useT } from "../../i18n";
import { lazy } from "../../i18n/lazy";
import { ChartContext, ChartDetails } from "./reporting.chartcontext";
import { chartTitle } from "./reporting.chartlabel";
import type {
  ReportingChart,
  ReportingEvaluation,
  ReportingEvidenceRef,
} from "./reporting.model";
import { useMetricNames } from "./reporting.queries";
import "./reporting.css";

const ChartBody = lazy(
  async () => ({
    default: (await import("./reporting.chartbody")).ChartBody,
  }),
  "complete",
  CardBoundary,
);
type EvidenceAction = (reference: ReportingEvidenceRef) => void;

export function ChartPanel({
  chart,
  wide,
  evaluation,
  editionId,
  section = false,
  meta,
  note,
  onEvidence,
}: Readonly<{
  chart: ReportingChart;
  wide: boolean;
  evaluation: ReportingEvaluation;
  editionId?: string;
  // A section of a larger sheet: its title sits flat over the chart.
  section?: boolean;
  // The chart's total, said beside its title in place of a headline above it.
  meta?: ReactNode;
  // What that total leaves out, said above the bars.
  note?: ReactNode;
  onEvidence: EvidenceAction;
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const drawn = chart.points.length > 0 && chart.coverage.status !== "no_data";
  const [tableSlot, setTableSlot] = useState<HTMLSpanElement | null>(null);
  return (
    <ChartTableSlotProvider value={tableSlot}>
      <Panel
        flatHead={section}
        className={
          wide ? "reporting-panel reporting-panel-wide" : "reporting-panel"
        }
        title={chartTitle(chart, evaluation, t, metricName)}
        titleAction={
          <>
            {meta}
            <ChartDetails chart={chart} evaluation={evaluation} />
          </>
        }
        footer={
          <>
            <span className="reporting-table-slot" ref={setTableSlot} />
            <ChartContext
              chart={chart}
              evaluation={evaluation}
              editionId={editionId}
            />
            {drawn && (
              <Button
                variant="link"
                onClick={() =>
                  onEvidence({
                    metric: chart.metric,
                    context_id: chart.context_id,
                  })
                }
              >
                {t("reporting.viewRecords")}
              </Button>
            )}
          </>
        }
      >
        <PanelBody>
          {/* A chart drawn from the reader's own figures alone says so above its
            bars: one bar on a team chart otherwise reads as a team of one. */}
          {drawn && chart.coverage.withheld && chart.coverage.reason && (
            <p className="t-caption">{chart.coverage.reason}</p>
          )}
          {drawn && note}
          {drawn ? (
            <Suspense fallback={<PendingBody label={t("common.loading")} />}>
              <ChartBody
                chart={chart}
                evaluation={evaluation}
                onEvidence={onEvidence}
                headline={meta == null}
              />
            </Suspense>
          ) : (
            <EmptyState>
              {chart.coverage.reason ?? t("common.empty")}
            </EmptyState>
          )}
        </PanelBody>
      </Panel>
    </ChartTableSlotProvider>
  );
}
