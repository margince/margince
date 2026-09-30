import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { useCan, useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { navigate, useRoute } from "../app/router";
import { Button, SegmentedControl } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";
import { executionLabel } from "./reporting.model";
import { useReportingPages } from "./reporting.pagination";
import { ReportingReportDetail } from "./reporting.report";

export function ReportingLibrary() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const canCreate = useCanWrite("report_definition", "create");
  const canSeeSchedules = useCan("report_schedule", "read");
  const [filter, setFilter] = useState("all");
  const route = useRoute();
  const { cursor, next: setCursor, back, canBack } = useReportingPages();
  const query = useQuery({
    queryKey: ["reporting-reports", cursor, filter],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/reports", {
        params: {
          query: {
            cursor,
            limit: 30,
            scheduled: filter === "scheduled" || undefined,
          },
        },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  if (route.id2)
    return (
      <ReportingReportDetail
        key={route.id2}
        reportId={route.id2}
        editionId={route.id3}
      />
    );
  return (
    <Panel
      title={t("reporting.reports")}
      titleAction={
        canCreate ? (
          <Button
            onClick={() => navigate({ screen: "analytics", id: "performance" })}
          >
            {t("reporting.createReport")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        {canSeeSchedules && (
          <SegmentedControl
            options={["all", "scheduled"]}
            value={filter}
            labels={{
              all: t("reporting.all"),
              scheduled: t("reporting.scheduled"),
            }}
            onChange={(next) => {
              setFilter(next);
              setCursor(undefined);
            }}
            label={t("reporting.reports")}
          />
        )}
        <QueryGate
          query={query}
          pendingLabel={t("reporting.reports")}
          empty={(result) => result.data.length === 0}
        >
          {(reports) => (
            <>
              <DataTable
                label={t("reporting.reports")}
                rows={reports.data}
                rowKey={(report) => report.id}
                columns={[
                  {
                    key: "name",
                    header: t("reporting.name"),
                    render: (report) => (
                      <>
                        <Button
                          variant="link"
                          onClick={() =>
                            navigate({
                              screen: "analytics",
                              id: "reports",
                              id2: report.id,
                            })
                          }
                        >
                          {report.name}
                        </Button>
                        <p className="t-sub">
                          {report.selection.scope.label} ·{" "}
                          {t(`reporting.${report.selection.period}`)}
                        </p>
                      </>
                    ),
                  },
                  {
                    key: "metrics",
                    header: t("reporting.metrics"),
                    render: (report) => report.selection.metrics.length,
                  },
                  {
                    key: "revision",
                    header: t("reporting.details"),
                    render: (report) =>
                      t("reporting.revision", {
                        revision: formatNumber(report.revision, locale),
                      }),
                  },
                  {
                    key: "editions",
                    header: t("reporting.editions"),
                    render: (report) => (
                      <>
                        {report.edition_count === undefined
                          ? "—"
                          : formatNumber(report.edition_count, locale)}
                        {report.latest_captured_at && (
                          <p className="t-sub">
                            {formatDateTime(
                              report.latest_captured_at,
                              locale,
                              zone,
                            )}
                          </p>
                        )}
                      </>
                    ),
                  },
                  {
                    key: "schedule",
                    header: t("reporting.schedule"),
                    render: (report) => (
                      <>
                        {report.cadence
                          ? report.cadence
                              .split(", ")
                              .map((frequency) =>
                                frequency === "weekly"
                                  ? t("reporting.weekly")
                                  : t("reporting.monthly"),
                              )
                              .join(", ")
                          : "—"}
                        {!!report.paused_schedule_count && (
                          <p className="t-sub">
                            {plural(
                              "reporting.pausedSchedules",
                              report.paused_schedule_count,
                              {
                                count: formatNumber(
                                  report.paused_schedule_count,
                                  locale,
                                ),
                              },
                            )}
                          </p>
                        )}
                        {report.next_due_at && (
                          <p className="t-sub">
                            {t("reporting.nextRun", {
                              at: formatDateTime(
                                report.next_due_at,
                                locale,
                                zone,
                              ),
                            })}
                          </p>
                        )}
                        {report.last_status && (
                          <p className="t-sub">
                            {executionLabel(report.last_status, t)}
                          </p>
                        )}
                      </>
                    ),
                  },
                  {
                    key: "audience",
                    header: t("reporting.audience"),
                    render: (report) => t(`reporting.${report.audience}`),
                  },
                ]}
              />
              {canBack && (
                <Button variant="ghost" onClick={back}>
                  {t("reporting.back")}
                </Button>
              )}
              {reports.next_cursor && (
                <Button
                  variant="ghost"
                  onClick={() => setCursor(reports.next_cursor)}
                >
                  {t("reporting.next")}
                </Button>
              )}
            </>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}
