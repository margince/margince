import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { downloadBytes } from "./download";
import { type ReportingEvaluation, reportingQuery } from "./reporting.model";

export function ReportingExportButton({
  evaluation,
  editionId,
}: Readonly<{
  evaluation: ReportingEvaluation;
  editionId?: string;
}>) {
  const t = useT();
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
    </>
  );
}
