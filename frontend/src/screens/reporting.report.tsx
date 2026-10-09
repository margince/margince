import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { useCan, useCanWrite } from "../app/capability";
import { navigate } from "../app/router";
import { Button, Disclosure, OverflowMenu } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";
import { ReportingCharts } from "./reporting.charts";
import { ReportingComparison } from "./reporting.comparison";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { ReportingExecutions } from "./reporting.executions";
import { ReportingExportButton } from "./reporting.export";
import {
  editionLabel,
  editionStatus,
  executionLabel,
  type ReportAction,
  type ReportingEvidenceRef,
  type ReportingReport,
} from "./reporting.model";
import { useReportDetailQueries } from "./reporting.queries";
import { SaveReportingDialog } from "./reporting.save";
import { ReportingScheduleDialog } from "./reporting.schedule";

export function ReportingReportDetail({
  reportId,
  editionId,
}: Readonly<{ reportId: string; editionId?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const [scheduleId, setScheduleId] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [comparing, setComparing] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [capturePending, setCapturePending] = useState<boolean | undefined>(
    undefined,
  );
  const [requestKey, setRequestKey] = useState(() => crypto.randomUUID());
  const canEdit = useCanWrite("report_definition", "update");
  const canCreate = useCanWrite("report_definition", "create");
  const canArchive = useCanWrite("report_definition", "delete");
  const canFreeze = useCanWrite("report_edition", "create");
  const canReadSchedules = useCan("report_schedule", "read");
  const canReadEditions = useCan("report_edition", "read");
  const canSchedule = useCanWrite("report_schedule", "create");
  const { report, live, edition, editions, schedules } = useReportDetailQueries(
    reportId,
    editionId,
    { editions: canReadEditions, schedules: canReadSchedules },
  );
  const write = useMutation({
    mutationFn: async (action: ReportAction) => {
      switch (action.kind) {
        case "duplicate": {
          const { data, error } = await api.POST("/analytics/reports", {
            body: {
              name: (action.name ?? action.report.name).slice(0, 160),
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
    onSuccess: async (result, action) => {
      if (action.kind === "freeze") setCapturePending(true);
      await client.invalidateQueries({ queryKey: ["reporting-reports"] });
      await client.invalidateQueries({
        queryKey: ["reporting-executions", reportId],
      });
      setRequestKey(crypto.randomUUID());
      navigate({ screen: "analytics", id: "reports", id2: result.reportId });
    },
  });
  const allEditions = editions.data?.pages.flatMap((page) => page.data) ?? [];
  const canManage = report.data?.can_manage === true;
  const canEditReport = canEdit && canManage;
  const canArchiveReport = canArchive && canManage;
  const canScheduleReport = canSchedule && canManage;
  const canFreezeReport = canFreeze && canReadEditions && canManage;
  const canCompare = allEditions.length >= 2;
  const evaluation = editionId ? edition.data?.evaluation : live.data;
  const evidencePanel =
    evidence && evaluation ? (
      <ReportingEvidenceDrawer
        key={`${editionId}:${JSON.stringify(evidence)}`}
        evaluation={evaluation}
        reference={evidence}
        editionId={editionId}
        onClose={() => setEvidence(null)}
      />
    ) : null;
  const openEdition = (id?: string) => {
    setEvidence(null);
    navigate({ screen: "analytics", id: "reports", id2: reportId, id3: id });
  };
  const snapshotOptions = [
    { value: "live", label: t("reporting.live") },
    ...(edition.data &&
    !allEditions.some((item) => item.id === edition.data?.id)
      ? [edition.data, ...allEditions]
      : allEditions
    ).map((item) => ({
      value: item.id,
      label: editionLabel(item, locale),
    })),
  ];
  const showHistory =
    historyOpen ||
    editions.isError ||
    schedules.isError ||
    capturePending === true;
  const reportActions = (report: ReportingReport) =>
    (canEditReport || canCreate || canArchiveReport) && (
      <OverflowMenu label={t("reporting.actions")}>
        {canEditReport && (
          <Button variant="ghost" onClick={() => setEditing(true)}>
            {t("reporting.edit")}
          </Button>
        )}
        {canCreate && (
          <Button
            variant="ghost"
            disabled={write.isPending}
            onClick={() =>
              write.mutate({
                kind: "duplicate",
                report,
                key: requestKey,
                name: t("reporting.copyName", { name: report.name }),
              })
            }
          >
            {t("reporting.duplicate")}
          </Button>
        )}
        {canArchiveReport && (
          <Button
            variant="ghost"
            disabled={write.isPending}
            onClick={() => setArchiving(true)}
          >
            {t("reporting.archive")}
          </Button>
        )}
      </OverflowMenu>
    );
  return (
    <QueryGate query={report} pendingLabel={t("reporting.reports")}>
      {(report) => (
        <>
          <div className="reporting-header">
            <div>
              <Heading as="h2" size="large">
                {report.name}
              </Heading>
              <p className="t-caption">
                {report.selection.scope.label} ·{" "}
                {t(`reporting.${report.selection.period}`)} ·{" "}
                {t(`reporting.${report.audience}`)}
              </p>
            </div>
            <div className="reporting-header-actions">
              {canReadEditions && (
                <Select
                  aria-label={t("reporting.editions")}
                  value={editionId ?? "live"}
                  options={snapshotOptions}
                  onChange={(value) =>
                    openEdition(value === "live" ? undefined : value)
                  }
                />
              )}
              {evaluation && (
                <ReportingExportButton
                  evaluation={evaluation}
                  editionId={editionId}
                />
              )}
              {reportActions(report)}
              {canScheduleReport && (
                <Button variant="ghost" onClick={() => setScheduleId("new")}>
                  {t("reporting.schedule")}
                </Button>
              )}
              {canFreezeReport && (
                <Button
                  disabled={write.isPending || capturePending !== false}
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
          {capturePending && (
            <p role="status">{t("reporting.capturePending")}</p>
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
                    · {editionStatus(edition, t)}
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
          <Disclosure
            summary={t("reporting.history")}
            open={showHistory}
            onToggle={setHistoryOpen}
          >
            {canReadSchedules && (
              <QueryGate
                query={schedules}
                pendingLabel={t("reporting.schedule")}
              >
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
                        · {schedule.timezone} ·{" "}
                        {executionLabel(schedule.last_status, t)}
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
                    empty={(result) =>
                      result.pages.every((page) => page.data.length === 0)
                    }
                  >
                    {() => (
                      <>
                        <DataTable
                          label={t("reporting.editions")}
                          rows={allEditions}
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
                                  {editionLabel(edition, locale)}
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
                        {editions.hasNextPage && (
                          <Button
                            variant="ghost"
                            pending={editions.isFetchingNextPage}
                            onClick={() => editions.fetchNextPage()}
                          >
                            {t("reporting.loadOlder")}
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          onClick={() => setComparing(true)}
                          disabled={!canCompare}
                        >
                          {t("reporting.compare")}
                        </Button>
                      </>
                    )}
                  </QueryGate>
                </PanelBody>
              </Panel>
            )}
            {canReadEditions && (
              <Panel title={t("reporting.executions")}>
                <PanelBody>
                  <ReportingExecutions
                    reportId={reportId}
                    canRetry={canFreeze}
                    key={requestKey}
                    onPendingChange={setCapturePending}
                  />
                </PanelBody>
              </Panel>
            )}
          </Disclosure>
          {evidencePanel}
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
              editions={allEditions}
              hasMore={editions.hasNextPage}
              loadingMore={editions.isFetchingNextPage}
              onLoadMore={() => editions.fetchNextPage()}
              onClose={() => setComparing(false)}
            />
          )}
        </>
      )}
    </QueryGate>
  );
}
