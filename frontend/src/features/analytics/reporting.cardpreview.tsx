import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { EmptyState, PendingBody } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { ChartPanel, pageCharts } from "./reporting.charts";
import {
  ReportingEvidenceDrawer,
  useSelectionEvidence,
} from "./reporting.evidence";
import type { CardChoice } from "./reporting.layout";
import { type ReportingSelection, reportingQuery } from "./reporting.model";

/**
 * One chart before it is added, evaluated as the report would be: the same
 * scope, period and metrics, under the editor's own grants. Nothing is saved.
 */
export function CardPreview({
  selection,
  choice,
}: Readonly<{ selection: ReportingSelection; choice: CardChoice }>) {
  const t = useT();
  const previewed: ReportingSelection = {
    ...selection,
    layout: undefined,
    blocks: [choice.kind],
    metrics: choice.metric ? [choice.metric] : selection.metrics,
  };
  const { evidence, openEvidence, closeEvidence } = useSelectionEvidence(
    JSON.stringify(previewed),
  );
  const query = useQuery({
    queryKey: ["reporting-card-preview", previewed],
    queryFn: async () =>
      unwrap(
        await api.GET("/analytics/evaluate", {
          params: { query: reportingQuery(previewed) },
        }),
      ),
  });
  if (query.isPending)
    return <PendingBody label={t("reporting.previewLoading")} />;
  if (query.isError) return <ErrorLine error={query.error} />;
  const evaluation = query.data;
  const charts = pageCharts(evaluation).filter(
    (chart) =>
      chart.kind === choice.kind &&
      (!choice.metric || chart.metric === choice.metric),
  );
  if (charts.length === 0)
    return <EmptyState>{t("reporting.previewEmpty")}</EmptyState>;
  return (
    <div className="form-stack">
      {charts.map((chart) => (
        <ChartPanel
          key={`${chart.kind}:${chart.metric}`}
          chart={chart}
          wide
          evaluation={evaluation}
          onEvidence={openEvidence}
        />
      ))}
      {evidence && (
        <ReportingEvidenceDrawer
          evaluation={evaluation}
          reference={evidence}
          onClose={closeEvidence}
        />
      )}
    </div>
  );
}
