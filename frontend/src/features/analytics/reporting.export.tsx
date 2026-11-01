import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { downloadBytes } from "../../lib/download";
import { isVersionSkewOf, throwProblem } from "../../lib/problem";
import { type ReportingEvaluation, reportingQuery } from "./reporting.model";
import { reloadReportingFigures } from "./reporting.queries";

/**
 * Downloads the server's CSV of an evaluation. `charts` is a second read of
 * the same question that carries its charts; its chart rows follow the
 * figures, each row under the receipt it was read with.
 */
export function ReportingExportButton({
  evaluation,
  charts,
  editionId,
}: Readonly<{
  evaluation: ReportingEvaluation;
  charts?: ReportingEvaluation;
  editionId?: string;
}>) {
  const t = useT();
  const client = useQueryClient();
  const download = useMutation({
    mutationFn: async ({
      evaluation,
      charts,
      editionId,
    }: {
      evaluation: ReportingEvaluation;
      charts?: ReportingEvaluation;
      editionId?: string;
    }) => {
      if (editionId) {
        const result = await api.GET("/analytics/editions/{id}/export.csv", {
          params: { path: { id: editionId } },
          parseAs: "text",
        });
        if (result.error) throwProblem(result.error);
        return result.data;
      }
      const figures = await evaluationCsv(evaluation);
      if (!charts) return figures;
      return joinChartRows(figures, await evaluationCsv(charts), charts);
    },
    onSuccess: (csv) =>
      downloadBytes(
        new Blob([csv], { type: "text/csv" }),
        "reporting.csv",
        "text/csv",
      ),
  });
  return (
    <>
      <Button
        variant="ghost"
        pending={download.isPending}
        onClick={() => download.mutate({ evaluation, charts, editionId })}
      >
        {t("reporting.exportCsv")}
      </Button>
      {isVersionSkewOf(download.error) ? (
        <ErrorLine
          actions={
            <Button
              onClick={() => {
                reloadReportingFigures(client);
                download.reset();
              }}
            >
              {t("common.retry")}
            </Button>
          }
        >
          {t("reporting.exportStale")}
        </ErrorLine>
      ) : (
        <ErrorLine error={download.error} />
      )}
    </>
  );
}

async function evaluationCsv(evaluation: ReportingEvaluation): Promise<string> {
  const result = await api.GET("/analytics/evaluate.csv", {
    params: {
      query: {
        ...reportingQuery(evaluation.selection),
        evaluation_key: evaluation.evaluation_key,
        evaluated_at: evaluation.context.evaluated_at,
        framework_revision: evaluation.context.framework_revision,
      },
    },
    parseAs: "text",
  });
  if (result.error) throwProblem(result.error);
  return result.data;
}

// The figures' CSV, then the chart rows of the chart read: the rows whose
// block column names one of the charts it answered. Its figure rows are the
// figures' own and are not repeated.
function joinChartRows(
  figures: string,
  chartCsv: string,
  charts: ReportingEvaluation,
): string {
  const kinds = new Set<string>(charts.charts.map((chart) => chart.kind));
  const rows = csvRecords(chartCsv).slice(1);
  const kept = rows.filter((row) => kinds.has(csvFields(row)[1] ?? ""));
  const ending = figures.endsWith("\n") ? "" : "\n";
  return figures + ending + kept.map((row) => `${row}\n`).join("");
}

// The records of a CSV as their raw lines, a quoted field's line breaks kept.
function csvRecords(csv: string): string[] {
  const records: string[] = [];
  let start = 0;
  let quoted = false;
  for (let index = 0; index < csv.length; index += 1) {
    const char = csv[index];
    if (char === '"') quoted = !quoted;
    else if (char === "\n" && !quoted) {
      records.push(csv.slice(start, index).replace(/\r$/, ""));
      start = index + 1;
    }
  }
  if (start < csv.length) records.push(csv.slice(start));
  return records;
}

// One record's fields, quotes undone.
function csvFields(record: string): string[] {
  const fields: string[] = [];
  let field = "";
  let quoted = false;
  for (let index = 0; index < record.length; index += 1) {
    const char = record[index];
    if (quoted && char === '"' && record[index + 1] === '"') {
      field += '"';
      index += 1;
    } else if (char === '"') quoted = !quoted;
    else if (char === "," && !quoted) {
      fields.push(field);
      field = "";
    } else field += char;
  }
  fields.push(field);
  return fields;
}
