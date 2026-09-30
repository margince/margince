import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button, Disclosure } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import { downloadBytes } from "./download";
import {
  metricLabel,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingAmount,
  reportingQuery,
} from "./reporting.model";

export function ReportingScorecard({
  evaluation,
  editionId,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  editionId?: string;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = evaluation.charts
    .filter((chart) => chart.kind === "owner_attainment")
    .flatMap((chart) =>
      chart.points.map((point) => ({
        metric: chart.metric,
        unit: chart.unit,
        ...point,
      })),
    );
  const download = useMutation({
    mutationFn: async ({
      evaluation,
      editionId,
    }: {
      evaluation: ReportingEvaluation;
      editionId?: string;
    }) => {
      const result = editionId
        ? await api.GET("/analytics/editions/{id}/export.csv", {
            params: { path: { id: editionId } },
            parseAs: "blob",
          })
        : await api.GET("/analytics/evaluate.csv", {
            params: {
              query: {
                ...reportingQuery(evaluation.selection),
                evaluation_key: evaluation.evaluation_key,
                evaluated_at: evaluation.context.evaluated_at,
                framework_revision: evaluation.context.framework_revision,
              },
            },
            parseAs: "blob",
          });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
    onSuccess: (blob) => downloadBytes(blob, "reporting.csv", "text/csv"),
  });
  return (
    <>
      <Button
        variant="ghost"
        pending={download.isPending}
        onClick={() => download.mutate({ evaluation, editionId })}
      >
        {t("reporting.exportCsv")}
      </Button>
      <ErrorLine error={download.error} />
      {rows.length > 0 && (
        <Disclosure summary={t("reporting.scorecard")}>
          <DataTable
            label={t("reporting.scorecard")}
            rows={rows}
            rowKey={(row) => `${row.metric}:${row.key}`}
            columns={[
              {
                key: "owner",
                header: t("reporting.owner"),
                render: (row) =>
                  row.evidence ? (
                    <Button
                      variant="link"
                      onClick={() => {
                        if (row.evidence) onEvidence(row.evidence);
                      }}
                    >
                      {row.label}
                    </Button>
                  ) : (
                    row.label
                  ),
              },
              {
                key: "metric",
                header: t("reporting.metrics"),
                render: (row) => metricLabel(row.metric, t),
              },
              {
                key: "actual",
                header: t("reporting.actual"),
                render: (row) =>
                  reportingAmount(
                    row.value,
                    row.unit,
                    evaluation.context.currency,
                    locale,
                  ),
              },
              {
                key: "target",
                header: t("reporting.target"),
                render: (row) =>
                  row.target == null
                    ? t("reporting.noTarget")
                    : reportingAmount(
                        row.target,
                        row.unit,
                        evaluation.context.currency,
                        locale,
                      ),
              },
            ]}
          />
        </Disclosure>
      )}
    </>
  );
}
