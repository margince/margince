import type { ReactNode } from "react";
import { Button, EmptyState } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { ChartBody, chartTitle } from "./reporting.chartbody";
import { ChartContext } from "./reporting.chartcontext";
import {
  type LaidOutItem,
  type LaidOutSection,
  laidOut,
} from "./reporting.layout";
import { MetricsTable } from "./reporting.metricstable";
import type {
  ReportingChart,
  ReportingEvaluation,
  ReportingEvidenceRef,
} from "./reporting.model";
import { type KeptQuestions, QuestionCard } from "./reporting.questioncard";
import { ResultsSummary } from "./reporting.summary";
import { WrittenSummary } from "./reporting.written";
import "./reporting.css";

type EvidenceAction = (reference: ReportingEvidenceRef) => void;

export function ReportingCharts({
  evaluation,
  editionId,
  kept,
  afterSummary,
  onEvidence,
  rolePreview,
}: Readonly<{
  evaluation: ReportingEvaluation;
  editionId?: string;
  // What a sent edition kept of its question cards.
  kept?: KeptQuestions;
  // Drawn between the headline readings and the charts: what a reader should
  // act on before reading further.
  afterSummary?: ReactNode;
  onEvidence: EvidenceAction;
  // Set while the page shows the report as a role reads it.
  rolePreview?: boolean;
}>) {
  const t = useT();
  const charts = pageCharts(evaluation);
  // Said once for the page: on every card it would repeat one fact many times.
  const withheld =
    evaluation.metrics.some((metric) => metric.coverage.withheld) ||
    evaluation.charts.some((chart) => chart.coverage.withheld);
  const sections: readonly LaidOutSection[] = evaluation.selection.layout
    ? laidOut(evaluation.selection.layout, charts)
    : [{ items: charts.map((chart) => ({ chart, full: false })) }];
  return (
    <div className="reporting-results">
      <WrittenSummary evaluation={evaluation} />
      <ResultsSummary evaluation={evaluation} onEvidence={onEvidence} />
      {withheld && <p className="t-caption">{t("reporting.withheld")}</p>}
      {afterSummary}
      {sections.map((section) => (
        <ChartSection
          // A card is placed once, so a section's first item names it.
          key={section.items[0] ? itemKey(section.items[0]) : "empty"}
          section={section}
          evaluation={evaluation}
          editionId={editionId}
          onEvidence={onEvidence}
          questions={questionMode(editionId, rolePreview)}
          kept={kept}
        />
      ))}
      <MetricsTable evaluation={evaluation} onEvidence={onEvidence} />
    </div>
  );
}

/**
 * The charts the page draws from an evaluation. A target chart with no target
 * is left out, and the accepted-opportunities series joins the meetings chart.
 * A preview reads the same list, so it shows what adding the card will.
 */
export function pageCharts(evaluation: ReportingEvaluation): ReportingChart[] {
  return evaluation.charts.filter(
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
}

// One section of the layout: its heading, when it has one, over its charts.
function ChartSection({
  section,
  evaluation,
  editionId,
  onEvidence,
  questions,
  kept,
}: Readonly<{
  section: LaidOutSection;
  evaluation: ReportingEvaluation;
  editionId?: string;
  onEvidence: EvidenceAction;
  questions: "live" | "frozen" | "preview";
  kept?: KeptQuestions;
}>) {
  const wide = pairWidths(
    section.items.map(
      (item) =>
        item.full || ("chart" in item && WIDE_KINDS.has(item.chart.kind)),
    ),
  );
  return (
    <section className="reporting-section">
      {section.title && (
        <Heading as="h3" size="small">
          {section.title}
        </Heading>
      )}
      <div className="reporting-grid">
        {section.items.map((item, index) =>
          "chart" in item ? (
            <ChartPanel
              key={itemKey(item)}
              chart={item.chart}
              wide={wide[index] === true}
              evaluation={evaluation}
              editionId={editionId}
              onEvidence={onEvidence}
            />
          ) : (
            <QuestionCard
              key={itemKey(item)}
              card={item.question}
              wide={wide[index] === true}
              evaluation={evaluation}
              mode={questions}
              kept={kept}
            />
          ),
        )}
      </div>
    </section>
  );
}

function questionMode(
  editionId: string | undefined,
  rolePreview: boolean | undefined,
): "live" | "frozen" | "preview" {
  if (editionId !== undefined) return "frozen";
  return rolePreview ? "preview" : "live";
}

function itemKey(item: LaidOutItem): string {
  return "chart" in item
    ? `${item.chart.kind}:${item.chart.metric}`
    : `question:${item.question.id}`;
}

// One chart in its panel. The head holds the title alone; what the figure
// covers and the door to the records behind it sit in the foot, so a row of
// cards reads as a row of titles over a row of charts.
export function ChartPanel({
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
      footer={
        <>
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
  return pairWidths(
    charts.map(
      (chart, index) => WIDE_KINDS.has(chart.kind) || full[index] === true,
    ),
  );
}

// Spans an unpaired half item across its row, given which items span already.
function pairWidths(spans: readonly boolean[]): boolean[] {
  const wide = [...spans];
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
