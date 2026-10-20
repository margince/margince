import type { ReactNode } from "react";
import { Button, EmptyState } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { ChartBody, chartTitle } from "./reporting.chartbody";
import { ChartContext } from "./reporting.chartcontext";
import { type LaidOutSection, laidOut } from "./reporting.layout";
import { MetricsTable } from "./reporting.metricstable";
import type {
  ReportingChart,
  ReportingEvaluation,
  ReportingEvidenceRef,
} from "./reporting.model";
import { ResultsSummary } from "./reporting.summary";
import { WrittenSummary } from "./reporting.written";
import "./reporting.css";

type EvidenceAction = (reference: ReportingEvidenceRef) => void;

export function ReportingCharts({
  evaluation,
  editionId,
  afterSummary,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  editionId?: string;
  // Drawn between the headline readings and the charts: what a reader should
  // act on before reading further.
  afterSummary?: ReactNode;
  onEvidence: EvidenceAction;
}>) {
  const charts = evaluation.charts.filter(
    (chart) =>
      chart.kind !== "metric_reading" &&
      (chart.kind !== "target_progress" ||
        chart.points.some((point) => point.target != null)) &&
      !(
        chart.kind === "sdr_outcomes" &&
        chart.metric === "accepted_opportunities" &&
        // The meetings chart draws both series, unless its own reading
        // failed; then the acceptance series keeps its own card.
        evaluation.charts.some(
          (other) =>
            other.kind === "sdr_outcomes" &&
            other.metric === "meetings_held" &&
            other.coverage.status !== "failed",
        )
      ),
  );
  const sections: readonly LaidOutSection[] = evaluation.selection.layout
    ? laidOut(evaluation.selection.layout, charts)
    : [{ charts, full: [] }];
  return (
    <div className="reporting-results">
      <WrittenSummary evaluation={evaluation} />
      <ResultsSummary evaluation={evaluation} onEvidence={onEvidence} />
      {afterSummary}
      {sections.map((section) => (
        <ChartSection
          // A chart is placed once, so a section's first chart names it.
          key={`${section.charts[0]?.kind}:${section.charts[0]?.metric}`}
          section={section}
          evaluation={evaluation}
          editionId={editionId}
          onEvidence={onEvidence}
        />
      ))}
      <MetricsTable evaluation={evaluation} onEvidence={onEvidence} />
    </div>
  );
}

// One section of the layout: its heading, when it has one, over its charts.
function ChartSection({
  section,
  evaluation,
  editionId,
  onEvidence,
}: Readonly<{
  section: LaidOutSection;
  evaluation: ReportingEvaluation;
  editionId?: string;
  onEvidence: EvidenceAction;
}>) {
  const wide = wideCharts(section.charts, section.full);
  return (
    <section className="reporting-section">
      {section.title && (
        <Heading as="h3" size="small">
          {section.title}
        </Heading>
      )}
      <div className="reporting-grid">
        {section.charts.map((chart, index) => (
          <ChartPanel
            key={`${chart.kind}:${chart.metric}`}
            chart={chart}
            wide={wide[index] === true}
            evaluation={evaluation}
            editionId={editionId}
            onEvidence={onEvidence}
          />
        ))}
      </div>
    </section>
  );
}

// One chart in its panel, with the door to the records behind it.
function ChartPanel({
  chart,
  wide,
  evaluation,
  editionId,
  onEvidence,
}: Readonly<{
  chart: ReportingChart;
  wide: boolean;
  evaluation: ReportingEvaluation;
  editionId?: string;
  onEvidence: EvidenceAction;
}>) {
  const t = useT();
  const drawn = chart.points.length > 0 && chart.coverage.status !== "no_data";
  return (
    <Panel
      className={
        wide ? "reporting-panel reporting-panel-wide" : "reporting-panel"
      }
      title={chartTitle(chart, evaluation, t)}
      // The one door from a chart to the records behind it, in the head
      // beside the title it opens, rather than trailing the figure.
      titleAction={
        drawn ? (
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
        ) : undefined
      }
    >
      <PanelBody>
        <ChartContext
          chart={chart}
          evaluation={evaluation}
          editionId={editionId}
        />
        {drawn ? (
          <ChartBody
            chart={chart}
            evaluation={evaluation}
            onEvidence={onEvidence}
          />
        ) : (
          <EmptyState>{chart.coverage.reason ?? t("common.empty")}</EmptyState>
        )}
      </PanelBody>
    </Panel>
  );
}

// Kinds that need the page's whole width. The waterfall's steps and the
// outcomes table each need more room than half a row.
const WIDE_KINDS: ReadonlySet<ReportingChart["kind"]> = new Set([
  "pipeline_movement",
  "sdr_outcomes",
]);

// Which charts span the whole row: a kind that needs it, or one its card makes
// full. A half-width chart with no partner beside it spans too, or the grid
// leaves the other half of its row empty.
export function wideCharts(
  charts: readonly ReportingChart[],
  full: readonly boolean[] = [],
): boolean[] {
  const wide = charts.map(
    (chart, index) => WIDE_KINDS.has(chart.kind) || full[index] === true,
  );
  let unpaired: number | null = null;
  for (const [index, isWide] of wide.entries()) {
    if (!isWide && unpaired == null) {
      unpaired = index;
      continue;
    }
    if (isWide && unpaired != null) wide[unpaired] = true;
    unpaired = null;
  }
  if (unpaired != null) wide[unpaired] = true;
  return wide;
}
