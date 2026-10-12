import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../api/client";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { QueryGate, unwrap } from "./common";
import { useReportingPages } from "./reporting.pagination";

export function ReportingExecutions({
  reportId,
  canRetry,
  onPendingChange,
}: Readonly<{
  reportId: string;
  canRetry: boolean;
  onPendingChange?: (pending: boolean) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const client = useQueryClient();
  const { cursor, next: setCursor, back, canBack } = useReportingPages();
  const latest = useQuery({
    queryKey: ["reporting-executions", reportId, undefined],
    queryFn: () => executionPage(reportId),
    refetchInterval: (query) =>
      query.state.data?.data.some(
        (execution) =>
          execution.status === "running" || execution.status === "pending",
      )
        ? 5000
        : false,
  });
  const history = useQuery({
    queryKey: ["reporting-executions", reportId, cursor],
    enabled: !!cursor,
    queryFn: () => executionPage(reportId, cursor),
  });
  const query = cursor ? history : latest;
  const pending = latest.data?.data.some(
    (execution) =>
      execution.status === "pending" || execution.status === "running",
  );
  useEffect(() => {
    if (pending !== undefined) onPendingChange?.(pending);
  }, [pending, onPendingChange]);
  const published = latest.data?.data
    .flatMap((execution) =>
      execution.edition_id ? [execution.edition_id] : [],
    )
    .join(":");
  useEffect(() => {
    if (published) {
      void client.invalidateQueries({ queryKey: ["reporting-reports"] });
      void client.invalidateQueries({
        queryKey: ["reporting-editions", reportId],
      });
    }
  }, [published, client, reportId]);
  const retry = useMutation({
    mutationFn: async (id: string) => {
      return unwrap(
        await api.POST("/analytics/executions/{id}/retry", {
          params: { path: { id } },
        }),
      );
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
                  render: (execution) =>
                    formatDateTime(execution.intended_due_at, locale, zone),
                },
                {
                  key: "status",
                  header: t("reporting.details"),
                  render: (execution) => (
                    <>
                      {t(`reporting.execution.${execution.status}`)}
                      {execution.reason && <p>{execution.reason}</p>}
                    </>
                  ),
                },
                {
                  key: "action",
                  header: t("common.retry"),
                  render: (execution) =>
                    execution.edition_id ? (
                      <Button
                        variant="link"
                        onClick={() =>
                          navigate({
                            screen: "analytics",
                            id: "reports",
                            id2: reportId,
                            id3: execution.edition_id,
                          })
                        }
                      >
                        {t("reporting.frozen")}
                      </Button>
                    ) : canRetry &&
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
            {canBack && (
              <Button variant="ghost" onClick={back}>
                {t("reporting.back")}
              </Button>
            )}
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

async function executionPage(reportId: string, cursor?: string) {
  return unwrap(
    await api.GET("/analytics/reports/{id}/executions", {
      params: { path: { id: reportId }, query: { cursor, limit: 5 } },
    }),
  );
}
