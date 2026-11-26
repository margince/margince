// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useCompanyZone } from "../../lib/companyzone";
import { scopeName } from "./analytics.context";
import type { ReportingReport } from "./reporting.model";

// The report's name, its purpose when it has one, and what it covers.
export function ReportTitle({ report }: Readonly<{ report: ReportingReport }>) {
  const t = useT();
  return (
    <div className="reporting-title">
      <Heading as="h2" size="large">
        {report.name}
      </Heading>
      {report.description && (
        <p className="reporting-description">{report.description}</p>
      )}
      <p className="t-caption">
        {scopeName(report.selection.scope, t)} ·{" "}
        {t(`reporting.${report.selection.period}`)} ·{" "}
        {t(`reporting.${report.audience}`)}
      </p>
    </div>
  );
}

// An archived report opened by its address says so, in place of the verbs
// that would give it new snapshots or schedules.
export function ArchivedReportNotice({
  report,
}: Readonly<{ report: ReportingReport }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useCompanyZone();
  if (!report.archived_at) return null;
  return (
    <Callout
      tone="info"
      kind="standing"
      title={t("reporting.archivedOn", {
        date: formatDate(report.archived_at, locale, zone),
      })}
    >
      {t("reporting.archivedBody")}
    </Callout>
  );
}

// What the reader may do to this report. An archived report takes no edits,
// snapshots or schedules, whatever the reader's role.
export function useReportVerbs(
  report: ReportingReport | undefined,
  canReadEditions: boolean,
) {
  const canEdit = useCanWrite("report_definition", "update");
  const canCreate = useCanWrite("report_definition", "create");
  const canArchive = useCanWrite("report_definition", "delete");
  const canFreeze = useCanWrite("report_edition", "create");
  const canSchedule = useCanWrite("report_schedule", "create");
  const canManage = report?.can_manage === true && !report.archived_at;
  return {
    canEdit,
    canCreate,
    canFreeze,
    canSchedule,
    canEditReport: canEdit && canManage,
    canArchiveReport: canArchive && canManage,
    canScheduleReport: canSchedule && canManage,
    canFreezeReport: canFreeze && canReadEditions && canManage,
  };
}
