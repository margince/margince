// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { EntityRef } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatDateTime, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import type { ReportingReport } from "./reporting.model";

type ReportRevision = components["schemas"]["ReportingReportRevision"];

export function reportRevisionsKey(reportId: string) {
  return ["reporting-revisions", reportId] as const;
}

/** Every revision of a report, newest first; only an editor may read them. */
export function useReportRevisions(reportId: string, enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: reportRevisionsKey(reportId),
    queryFn: async () =>
      unwrap(
        await api.GET("/analytics/reports/{id}/revisions", {
          params: { path: { id: reportId } },
        }),
      ),
  });
}

/** The selection an open draft holds, once the revisions have loaded. */
export function draftRevision(
  revisions: readonly ReportRevision[] | undefined,
): ReportRevision | undefined {
  return revisions?.find((revision) => revision.state === "draft");
}

type DraftAction =
  | Readonly<{ kind: "publish" | "discard" }>
  | Readonly<{ kind: "restore"; revision: number }>;

// Every change to a report's revisions moves what the library, the report,
// its live figures and its revision list show.
function useRevisionAction(report: ReportingReport) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (action: DraftAction) => {
      const params = { path: { id: report.id }, ...ifMatch(report.version) };
      switch (action.kind) {
        case "publish":
          return unwrap(
            await api.POST("/analytics/reports/{id}/draft/publish", { params }),
          );
        case "discard":
          return unwrap(
            await api.DELETE("/analytics/reports/{id}/draft", { params }),
          );
        case "restore":
          return unwrap(
            await api.POST(
              "/analytics/reports/{id}/revisions/{revision}/restore",
              {
                params: {
                  ...params,
                  path: { id: report.id, revision: action.revision },
                },
              },
            ),
          );
      }
    },
    onSuccess: async () => {
      await Promise.all(
        [
          ["reporting-reports"],
          ["reporting-report", report.id],
          ["reporting-live", report.id],
          reportRevisionsKey(report.id),
        ].map((queryKey) => client.invalidateQueries({ queryKey })),
      );
    },
  });
}

/**
 * The editor's open draft: readers still see the published revision, so the
 * bar says so and offers to preview, publish or discard it.
 */
export function ReportDraftBar({
  report,
  previewing,
  onPreview,
}: Readonly<{
  report: ReportingReport;
  previewing: boolean;
  onPreview: (previewing: boolean) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const action = useRevisionAction(report);
  if (report.draft_revision === undefined) {
    return null;
  }
  return (
    <Callout
      tone="info"
      kind="standing"
      title={t("reporting.draftOpen", {
        revision: formatNumber(report.draft_revision, locale),
      })}
    >
      <p>
        {t(previewing ? "reporting.draftPreviewing" : "reporting.draftHidden")}
      </p>
      <div className="reporting-header-actions">
        <Button variant="ghost" onClick={() => onPreview(!previewing)}>
          {t(previewing ? "reporting.showPublished" : "reporting.previewDraft")}
        </Button>
        <Button
          disabled={action.isPending}
          onClick={() =>
            action.mutate(
              { kind: "publish" },
              { onSuccess: () => onPreview(false) },
            )
          }
        >
          {t("reporting.publishDraft")}
        </Button>
        <Button
          variant="ghost"
          disabled={action.isPending}
          onClick={() =>
            action.mutate(
              { kind: "discard" },
              { onSuccess: () => onPreview(false) },
            )
          }
        >
          {t("reporting.discardDraft")}
        </Button>
      </div>
      <ErrorLine error={action.error} />
    </Callout>
  );
}

/**
 * Who saved each revision and when. An earlier one can be published again as
 * the newest; editions keep the revision they were made from.
 */
export function ReportVersions({
  report,
}: Readonly<{ report: ReportingReport }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const revisions = useReportRevisions(report.id, true);
  const action = useRevisionAction(report);
  const draftOpen = report.draft_revision !== undefined;
  return (
    <Panel title={t("reporting.versions")}>
      <PanelBody>
        <QueryGate query={revisions} pendingLabel={t("reporting.versions")}>
          {(result) => (
            <DataTable
              label={t("reporting.versions")}
              rows={result.data}
              rowKey={(revision) => String(revision.revision)}
              columns={[
                {
                  key: "revision",
                  header: t("reporting.versions"),
                  render: (revision) =>
                    `${t("reporting.revision", {
                      revision: formatNumber(revision.revision, locale),
                    })} · ${t(`reporting.revisionState.${revision.state}`)}`,
                },
                {
                  key: "saved",
                  header: t("reporting.savedBy"),
                  render: (revision) => (
                    <>
                      <EntityRef kind="user" id={revision.created_by} /> ·{" "}
                      {formatDateTime(revision.created_at, locale, zone)}
                    </>
                  ),
                },
                {
                  key: "restore",
                  header: t("table.actions"),
                  headerHidden: true,
                  render: (revision) =>
                    revision.state === "earlier" && (
                      <Button
                        variant="ghost"
                        disabled={draftOpen || action.isPending}
                        onClick={() =>
                          action.mutate({
                            kind: "restore",
                            revision: revision.revision,
                          })
                        }
                      >
                        {t("reporting.restoreRevision", {
                          revision: formatNumber(revision.revision, locale),
                        })}
                      </Button>
                    ),
                },
              ]}
            />
          )}
        </QueryGate>
        {draftOpen && (
          <p className="t-caption">{t("reporting.restoreBlocked")}</p>
        )}
        <ErrorLine error={action.error} />
      </PanelBody>
    </Panel>
  );
}
