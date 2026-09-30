import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";

export function ReportingExecutions({
  reportId,
  canRetry,
}: Readonly<{ reportId: string; canRetry: boolean }>) {
  const t = useT();
  const client = useQueryClient();
  const [cursor, setCursor] = useState<string>();
  const query = useQuery({
    queryKey: ["reporting-executions", reportId, cursor],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/analytics/reports/{id}/executions",
        { params: { path: { id: reportId }, query: { cursor, limit: 5 } } },
      );
      if (error) throwProblem(error);
      return data;
    },
    refetchInterval: (query) =>
      query.state.data?.data.some(
        (execution) =>
          execution.status === "running" || execution.status === "pending",
      )
        ? 5000
        : false,
  });
  const published = query.data?.data
    .flatMap((execution) =>
      execution.edition_id ? [execution.edition_id] : [],
    )
    .join(":");
  useEffect(() => {
    if (published) {
      void client.invalidateQueries({
        queryKey: ["reporting-editions", reportId],
      });
    }
  }, [published, client, reportId]);
  const retry = useMutation({
    mutationFn: async (id: string) => {
      const { data, error } = await api.POST(
        "/analytics/executions/{id}/retry",
        { params: { path: { id } } },
      );
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: async () => {
      await client.invalidateQueries({
        queryKey: ["reporting-executions", reportId],
      });
    },
  });
  return (
    <>
      <QueryGate query={query} pendingLabel={t("reporting.executions")}>
        {(result) => (
          <>
            <DataTable
              label={t("reporting.executions")}
              rows={result.data}
              rowKey={(execution) => execution.id}
              columns={[
                {
                  key: "date",
                  header: t("reporting.period"),
                  render: (execution) => execution.intended_due_at,
                },
                {
                  key: "status",
                  header: t("reporting.details"),
                  render: (execution) =>
                    `${execution.status} · ${execution.reason ?? ""}`,
                },
                {
                  key: "action",
                  header: t("common.retry"),
                  render: (execution) =>
                    canRetry &&
                    ["failed", "suspended"].includes(execution.status) ? (
                      <Button
                        variant="ghost"
                        disabled={retry.isPending}
                        onClick={() => retry.mutate(execution.id)}
                      >
                        {t("common.retry")}
                      </Button>
                    ) : null,
                },
              ]}
            />
            {result.next_cursor && (
              <Button
                variant="ghost"
                onClick={() => setCursor(result.next_cursor)}
              >
                {t("reporting.next")}
              </Button>
            )}
          </>
        )}
      </QueryGate>
      <ErrorLine error={retry.error} />
    </>
  );
}
