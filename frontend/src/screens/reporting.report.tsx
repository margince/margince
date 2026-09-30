import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { useCan, useCanWrite } from "../app/capability";
import { navigate } from "../app/router";
import { Button, Disclosure } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { BarList } from "../design-system/readings";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";
import { ReportingCharts } from "./reporting.charts";
import { ReportingComparison } from "./reporting.comparison";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { ReportingExecutions } from "./reporting.executions";
import {
  type ReportingEvidenceRef,
  type ReportingReport,
  reportingAmount,
} from "./reporting.model";
import { SaveReportingDialog } from "./reporting.save";
import { ReportingScheduleDialog } from "./reporting.schedule";

type ReportAction = {
  kind: "duplicate" | "archive" | "freeze";
  report: ReportingReport;
  key: string;
};
export function ReportingReportDetail({
  reportId,
  editionId,
}: Readonly<{ reportId: string; editionId?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const [cursor, setCursor] = useState<string>();
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const [scheduleId, setScheduleId] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [comparing, setComparing] = useState(false);
  const [requestKey, setRequestKey] = useState(() => crypto.randomUUID());
  const canEdit = useCanWrite("report_definition", "update");
  const canCreate = useCanWrite("report_definition", "create");
  const canArchive = useCanWrite("report_definition", "delete");
  const canFreeze = useCanWrite("report_edition", "create");
  const canReadSchedules = useCan("report_schedule", "read");
  const canReadEditions = useCan("report_edition", "read");
  const canSchedule = useCanWrite("report_schedule", "create");
  const report = useQuery({
    queryKey: ["reporting-report", reportId],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/reports/{id}", {
        params: { path: { id: reportId } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const live = useQuery({
    enabled: !editionId,
    queryKey: ["reporting-live", reportId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/analytics/reports/{id}/evaluation",
        { params: { path: { id: reportId } } },
      );
      if (error) throwProblem(error);
      return data;
    },
  });
  const edition = useQuery({
    enabled: !!editionId,
    queryKey: ["reporting-edition", editionId],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/editions/{id}", {
        params: { path: { id: editionId ?? "" } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const editions = useQuery({
    enabled: canReadEditions,
    queryKey: ["reporting-editions", reportId, cursor],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/analytics/reports/{id}/editions",
        { params: { path: { id: reportId }, query: { cursor, limit: 5 } } },
      );
      if (error) throwProblem(error);
      return data;
    },
  });
  const schedules = useQuery({
    enabled: canReadSchedules,
    queryKey: ["reporting-schedules", reportId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/analytics/reports/{id}/schedules",
        { params: { path: { id: reportId } } },
      );
      if (error) throwProblem(error);
      return data;
    },
  });
  const write = useMutation({
    mutationFn: async (action: ReportAction) => {
      switch (action.kind) {
        case "duplicate": {
          const { data, error } = await api.POST("/analytics/reports", {
            body: {
              name: action.report.name,
              audience: "private",
              selection: action.report.selection,
            },
          });
          if (error) throwProblem(error);
          return { reportId: data.id };
        }
        case "archive": {
          const { error } = await api.DELETE("/analytics/reports/{id}", {
            params: { path: { id: action.report.id } },
          });
          if (error) throwProblem(error);
          return {};
        }
        case "freeze": {
          const { error } = await api.POST("/analytics/reports/{id}/editions", {
            params: {
              path: { id: action.report.id },
              header: { "Idempotency-Key": action.key },
            },
          });
          if (error) throwProblem(error);
          return { reportId: action.report.id };
        }
      }
    },
    onSuccess: async (result) => {
      await client.invalidateQueries({ queryKey: ["reporting-reports"] });
      await client.invalidateQueries({
        queryKey: ["reporting-executions", reportId],
      });
      setRequestKey(crypto.randomUUID());
      navigate({ screen: "analytics", id: "reports", id2: result.reportId });
    },
  });
  const evaluation = editionId ? edition.data?.evaluation : live.data;
  const openEdition = (id?: string) => {
    setEvidence(null);
    navigate({ screen: "analytics", id: "reports", id2: reportId, id3: id });
  };
  return (
    <QueryGate query={report} pendingLabel={t("reporting.reports")}>
      {(report) => (
        <>
          <div className="reporting-header">
            <div>
              <Heading as="h2" size="large">
                {report.name}
              </Heading>
              <p>
                {t("reporting.revision", {
                  revision: formatNumber(report.revision, locale),
                })}{" "}
                · {t(`reporting.${report.audience}`)}
              </p>
            </div>
            <div className="reporting-header-actions">
              <Button variant="ghost" onClick={() => openEdition()}>
                {t("reporting.live")}
              </Button>
              {canEdit && report.can_manage && (
                <Button variant="ghost" onClick={() => setEditing(true)}>
                  {t("reporting.edit")}
                </Button>
              )}
              {canCreate && (
                <Button
                  variant="ghost"
                  disabled={write.isPending}
                  onClick={() =>
                    write.mutate({ kind: "duplicate", report, key: requestKey })
                  }
                >
                  {t("reporting.duplicate")}
                </Button>
              )}
              {canArchive && report.can_manage && (
                <Button
                  variant="ghost"
                  disabled={write.isPending}
                  onClick={() => setArchiving(true)}
                >
                  {t("reporting.archive")}
                </Button>
              )}
              {canSchedule && report.can_manage && (
                <Button variant="ghost" onClick={() => setScheduleId("new")}>
                  {t("reporting.schedule")}
                </Button>
              )}
              {canFreeze && report.can_manage && (
                <Button
                  disabled={write.isPending}
                  onClick={() =>
                    write.mutate({ kind: "freeze", report, key: requestKey })
                  }
                >
                  {t("reporting.freeze")}
                </Button>
              )}
            </div>
          </div>
          <ConfirmModal
            open={archiving}
            onClose={() => setArchiving(false)}
            title={t("reporting.archive")}
            confirmLabel={t("reporting.archive")}
            confirmVariant="danger"
            pending={write.isPending}
            onConfirm={() =>
              write.mutate({ kind: "archive", report, key: requestKey })
            }
          >
            <p>{t("reporting.archiveConfirm")}</p>
            <ErrorLine error={write.error} />
          </ConfirmModal>
          <ErrorLine error={write.error} />
          {canReadSchedules && (
            <QueryGate query={schedules} pendingLabel={t("reporting.schedule")}>
              {(result) => (
                <>
                  {result.data.map((schedule) => (
                    <p key={schedule.id}>
                      <Button
                        variant="link"
                        onClick={() => setScheduleId(schedule.id)}
                        disabled={!canSchedule}
                      >
                        {t(`reporting.${schedule.definition.frequency}`)} ·{" "}
                        {t("reporting.revision", {
                          revision: formatNumber(
                            schedule.definition.report_revision,
                            locale,
                          ),
                        })}
                      </Button>{" "}
                      ·{" "}
                      {schedule.definition.enabled
                        ? t("reporting.nextRun", {
                            at: formatDateTime(
                              schedule.next_due_at,
                              locale,
                              schedule.timezone,
                            ),
                          })
                        : t("reporting.pause")}{" "}
                      · {schedule.timezone} · {schedule.last_status}
                    </p>
                  ))}
                </>
              )}
            </QueryGate>
          )}
          {canReadEditions && (
            <Panel title={t("reporting.editions")}>
              <PanelBody>
                <QueryGate
                  query={editions}
                  pendingLabel={t("reporting.editions")}
                  empty={(result) => result.data.length === 0}
                >
                  {(result) => (
                    <>
                      <BarList
                        label={t("reporting.editions")}
                        onSelect={openEdition}
                        rows={result.data.flatMap((edition) => {
                          const reading = edition.evaluation.metrics.find(
                            (metric) => metric.id === "bookings_won",
                          );
                          return reading?.value == null
                            ? []
                            : [
                                {
                                  key: edition.id,
                                  label: formatDateTime(
                                    edition.intended_due_at,
                                    locale,
                                    edition.evaluation.context.timezone,
                                  ),
                                  value: reading.value,
                                  amount: reportingAmount(
                                    reading.value,
                                    reading.unit,
                                    edition.evaluation.context.currency,
                                    locale,
                                  ),
                                },
                              ];
                        })}
                      />
                      <DataTable
                        label={t("reporting.editions")}
                        rows={result.data}
                        rowKey={(edition) => edition.id}
                        columns={[
                          {
                            key: "date",
                            header: t("reporting.period"),
                            render: (edition) => (
                              <Button
                                variant="link"
                                onClick={() => openEdition(edition.id)}
                              >
                                {formatDateTime(
                                  edition.intended_due_at,
                                  locale,
                                  edition.evaluation.context.timezone,
                                )}
                              </Button>
                            ),
                          },
                          {
                            key: "version",
                            header: t("reporting.details"),
                            render: (edition) =>
                              t("reporting.revision", {
                                revision: formatNumber(
                                  edition.report_revision,
                                  locale,
                                ),
                              }),
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
                      <Button
                        variant="ghost"
                        onClick={() => setComparing(true)}
                        disabled={result.data.length < 2}
                      >
                        {t("reporting.compare")}
                      </Button>
                    </>
                  )}
                </QueryGate>
              </PanelBody>
            </Panel>
          )}
          {editionId ? (
            <QueryGate query={edition} pendingLabel={t("reporting.frozen")}>
              {(edition) => (
                <>
                  <p className="reporting-archive-banner" role="status">
                    {t("reporting.frozen")} ·{" "}
                    {formatDateTime(
                      edition.captured_at,
                      locale,
                      edition.evaluation.context.timezone,
                    )}{" "}
                    ·{" "}
                    {edition.expired
                      ? t("reporting.expired")
                      : edition.redacted
                        ? t("reporting.redacted")
                        : edition.withheld
                          ? t("reporting.withheld")
                          : edition.name}
                  </p>
                  <ReportingCharts
                    editionId={editionId}
                    evaluation={edition.evaluation}
                    onEvidence={setEvidence}
                  />
                </>
              )}
            </QueryGate>
          ) : (
            <QueryGate query={live} pendingLabel={t("reporting.live")}>
              {(evaluation) => (
                <>
                  <p className="t-caption">{t("reporting.live")}</p>
                  <ReportingCharts
                    editionId={editionId}
                    evaluation={evaluation}
                    onEvidence={setEvidence}
                  />
                </>
              )}
            </QueryGate>
          )}
          {canReadEditions && (
            <Disclosure summary={t("reporting.executions")}>
              <ReportingExecutions reportId={reportId} canRetry={canFreeze} />
            </Disclosure>
          )}
          {evidence && evaluation && (
            <ReportingEvidenceDrawer
              key={`${editionId}:${JSON.stringify(evidence)}`}
              evaluation={evaluation}
              reference={evidence}
              editionId={editionId}
              onClose={() => setEvidence(null)}
            />
          )}
          {editing && (
            <SaveReportingDialog
              report={report}
              selection={report.selection}
              onClose={() => setEditing(false)}
              onSaved={() => setEditing(false)}
            />
          )}
          {scheduleId && evaluation && (
            <ReportingScheduleDialog
              report={report}
              schedule={schedules.data?.data.find(
                (schedule) => schedule.id === scheduleId,
              )}
              timezone={evaluation.context.timezone}
              onClose={() => setScheduleId(null)}
            />
          )}
          {comparing && (
            <ReportingComparison
              editions={editions.data?.data ?? []}
              onClose={() => setComparing(false)}
            />
          )}
        </>
      )}
    </QueryGate>
  );
}
