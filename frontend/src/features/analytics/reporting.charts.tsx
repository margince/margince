import type { ReactNode } from "react";
import { Button, EmptyState } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { AnalyticsBoard, AnalyticsSheet } from "./analytics.board";
import { AssessmentCard } from "./reporting.assessmentcard";
import { ChartBody, chartTitle } from "./reporting.chartbody";
import { ChartContext, ChartDetails } from "./reporting.chartcontext";
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
  ReportingSelection,
} from "./reporting.model";
import { useMetricNames, useReportingCatalog } from "./reporting.queries";
import {
  type KeptQuestions,
  QuestionCard,
  type ReportRevision,
} from "./reporting.questioncard";
import { ResultsSummary } from "./reporting.summary";
import { TargetComparison } from "./reporting.targetcomparison";
import { WrittenSummary } from "./reporting.written";
import "./reporting.css";

type EvidenceAction = (reference: ReportingEvidenceRef) => void;

export function ReportingCharts({
  evaluation,
  report,
  editionId,
  kept,
  afterSummary,
  onEvidence,
  onApplySelection,
  rolePreview,
  board = false,
}: Readonly<{
  evaluation: ReportingEvaluation;
  // The saved report and revision this evaluation reads, which a bound
  // question card asks the server about.
  report?: ReportRevision;
  editionId?: string;
  // What a sent edition kept of its question cards.
  kept?: KeptQuestions;
  // Drawn between the headline readings and the charts: what a reader should
  // act on before reading further.
  afterSummary?: ReactNode;
  onEvidence: EvidenceAction;
  // Switches the page to a selection the server offers, where it can.
  onApplySelection?: (selection: ReportingSelection) => void;
  // Set while the page shows the report as a role reads it.
  rolePreview?: boolean;
  // An Analytics tab: figures on one sheet, progress to target in the rail
  // beside it. "sheet" draws the sheet's sections alone, for a sheet the page
  // already holds. A saved layout keeps its own sections.
  board?: boolean | "sheet";
}>) {
  const t = useT();
  // Every card names its metric from the catalog, so the page waits for it
  // rather than show an id.
  const catalog = useReportingCatalog();
  const charts = pageCharts(evaluation);
  // Said once for the page: on every card it would repeat one fact many times.
  const withheld =
    evaluation.metrics.some((metric) => metric.coverage.withheld) ||
    evaluation.charts.some((chart) => chart.coverage.withheld);
  const sections: readonly LaidOutSection[] = evaluation.selection.layout
    ? laidOut(evaluation.selection.layout, charts)
    : [{ items: charts.map((chart) => ({ chart, full: false })) }];
  // The summary is written once: on its card where the layout places one.
  const placesAssessment = sections.some((section) =>
    section.items.some((item) => "assessment" in item),
  );
  return (
    <QueryGate query={catalog} pendingLabel={t("reporting.metrics")}>
      {() => (
        <div
          className={
            board === "sheet" ? "reporting-results-part" : "reporting-results"
          }
        >
          {!placesAssessment && <WrittenSummary evaluation={evaluation} />}
          {board && !evaluation.selection.layout ? (
            <BoardResults
              inSheet={board === "sheet"}
              evaluation={evaluation}
              charts={charts}
              withheld={withheld}
              afterSummary={afterSummary}
              onEvidence={onEvidence}
              onApplySelection={onApplySelection}
            />
          ) : (
            <>
              <ResultsSummary evaluation={evaluation} onEvidence={onEvidence} />
              <TargetComparison
                evaluation={evaluation}
                onApply={onApplySelection}
              />
              {withheld && (
                <p className="t-caption">{t("reporting.withheld")}</p>
              )}
              {afterSummary}
              {sections.map((section) => (
                <ChartSection
                  // A card is placed once, so a section's first item names it.
                  key={section.items[0] ? itemKey(section.items[0]) : "empty"}
                  section={section}
                  evaluation={evaluation}
                  report={report}
                  editionId={editionId}
                  onEvidence={onEvidence}
                  questions={questionMode(editionId, rolePreview)}
                  kept={kept}
                />
              ))}
              <MetricsTable evaluation={evaluation} onEvidence={onEvidence} />
            </>
          )}
        </div>
      )}
    </QueryGate>
  );
}

// The board form of one evaluation. Readings, charts and the metrics table make
// one sheet; progress to target sits in the rail with anything to act on first.
function BoardResults({
  inSheet,
  evaluation,
  charts,
  withheld,
  afterSummary,
  onEvidence,
  onApplySelection,
}: Readonly<{
  // Drawn into a sheet the page holds: no rail, so every chart stays here.
  inSheet: boolean;
  evaluation: ReportingEvaluation;
  charts: readonly ReportingChart[];
  withheld: boolean;
  afterSummary?: ReactNode;
  onEvidence: EvidenceAction;
  onApplySelection?: (selection: ReportingSelection) => void;
}>) {
  const t = useT();
  const railed = inSheet
    ? []
    : charts.filter((chart) => RAIL_KINDS.has(chart.kind));
  const sheet = charts.filter((chart) => !railed.includes(chart));
  // A split by group pairs with the next one; every other chart takes the row.
  const wide = wideCharts(
    sheet,
    sheet.map((chart) => chart.kind !== "breakdown"),
  );
  const rail =
    (afterSummary != null && afterSummary !== false) || railed.length > 0 ? (
      <>
        {afterSummary}
        {railed.map((chart) => (
          <ChartPanel
            key={`${chart.kind}:${chart.metric}`}
            chart={chart}
            wide
            section
            evaluation={evaluation}
            onEvidence={onEvidence}
          />
        ))}
      </>
    ) : undefined;
  const sections = (
    <>
      <ResultsSummary
        evaluation={evaluation}
        density="plate"
        lead={!inSheet}
        onEvidence={onEvidence}
      />
      <TargetComparison evaluation={evaluation} onApply={onApplySelection} />
      {withheld && <p className="t-caption">{t("reporting.withheld")}</p>}
      {/* On a sheet with no rail, what follows the summary stays on the sheet. */}
      {inSheet && afterSummary}
      {sheet.length > 0 && (
        <div className="reporting-grid reporting-sheet-grid">
          {sheet.map((chart, index) => (
            <ChartPanel
              key={`${chart.kind}:${chart.metric}`}
              chart={chart}
              wide={wide[index] === true}
              section
              evaluation={evaluation}
              onEvidence={onEvidence}
            />
          ))}
        </div>
      )}
      <MetricsTable evaluation={evaluation} onEvidence={onEvidence} />
    </>
  );
  if (inSheet) return sections;
  return (
    <AnalyticsBoard
      railLabel={t("analytics.board.rail")}
      rail={rail}
      main={<AnalyticsSheet>{sections}</AnalyticsSheet>}
    />
  );
}

// Charts read against a target per person or per figure: narrow lists that
// read beside the figures rather than across the sheet.
const RAIL_KINDS: ReadonlySet<ReportingChart["kind"]> = new Set([
  "owner_attainment",
  "target_progress",
]);

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
  report,
  editionId,
  onEvidence,
  questions,
  kept,
}: Readonly<{
  section: LaidOutSection;
  evaluation: ReportingEvaluation;
  report?: ReportRevision;
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
          ) : "assessment" in item ? (
            <AssessmentCard
              key={itemKey(item)}
              evaluation={evaluation}
              wide={wide[index] === true}
            />
          ) : (
            <QuestionCard
              key={itemKey(item)}
              card={item.question}
              wide={wide[index] === true}
              evaluation={evaluation}
              report={report}
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
  if ("chart" in item) return `${item.chart.kind}:${item.chart.metric}`;
  if ("assessment" in item) return `assessment:${item.assessment.id}`;
  return `question:${item.question.id}`;
}

// One chart in its panel. The head holds the title and one quiet door to what
// the figure was read from. What it covers and the door to its records share
// one foot line, so a row of cards reads as titles over charts.
export function ChartPanel({
  chart,
  wide,
  evaluation,
  editionId,
  section = false,
  onEvidence,
}: Readonly<{
  chart: ReportingChart;
  wide: boolean;
  evaluation: ReportingEvaluation;
  editionId?: string;
  // A section of a larger sheet: its title sits flat over the chart.
  section?: boolean;
  onEvidence: EvidenceAction;
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const drawn = chart.points.length > 0 && chart.coverage.status !== "no_data";
  return (
    <Panel
      flatHead={section}
      className={
        wide ? "reporting-panel reporting-panel-wide" : "reporting-panel"
      }
      title={chartTitle(chart, evaluation, t, metricName)}
      titleAction={<ChartDetails chart={chart} evaluation={evaluation} />}
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
        {/* A chart drawn from the reader's own figures alone says so above its
            bars: one bar on a team chart otherwise reads as a team of one. */}
        {drawn && chart.coverage.withheld && chart.coverage.reason && (
          <p className="t-caption">{chart.coverage.reason}</p>
        )}
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
