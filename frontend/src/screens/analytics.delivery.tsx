// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ENTITY } from "../app/entity";
import { routeHash } from "../app/router";
import { EmptyState } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import {
  formatDateTime,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { type Locale, useT } from "../i18n";
import {
  BarFigure,
  columnScale,
  type ReportRow,
  rowCount,
  rowMoney,
} from "./analytics.cells";
import { CellExplain, rowDerivationUrl } from "./analytics.explain";
import { isProjectPhase, PHASE_LABEL } from "./projects.form";

// The delivery section's three tables: what was sold becoming what is
// delivered, from the three project reports.

type PhaseRow = {
  phase: string;
  projects: number;
  openMinor: number | null;
  wonMinor: number | null;
  derivationUrl: string | null;
};

// Projects per phase, with the deal money standing behind each phase — both
// server-converted sums; this file only formats them.
export function ProjectsByPhaseTable({
  rows,
  locale,
  baseCurrency,
}: Readonly<{
  rows: ReportRow[];
  locale: Locale;
  baseCurrency: string | null;
}>) {
  const t = useT();
  const phased: PhaseRow[] = rows
    .filter((row) => typeof row.phase === "string")
    .map((row) => ({
      phase: String(row.phase),
      projects: rowCount(row, "projects"),
      openMinor: rowMoney(row, "open_deal_value_minor"),
      wonMinor: rowMoney(row, "won_deal_value_minor"),
      derivationUrl: rowDerivationUrl(row),
    }));
  if (phased.length === 0) {
    return <EmptyState>{t("analytics.noProjectsYet")}</EmptyState>;
  }
  const scale = columnScale(phased.map((row) => row.projects));
  return (
    <DataTable
      label={t("analytics.reportProjectsByPhase")}
      columns={[
        {
          key: "phase",
          header: t("project.phaseLabel"),
          render: (row: PhaseRow) => (
            <CellExplain
              url={row.derivationUrl}
              figure={phaseLabel(row.phase, t)}
            >
              {phaseLabel(row.phase, t)}
            </CellExplain>
          ),
        },
        {
          key: "projects",
          header: t("analytics.projects"),
          align: "end",
          grow: true,
          render: (row: PhaseRow) => (
            <BarFigure
              value={row.projects}
              scale={scale}
              label={t("analytics.projects")}
            >
              {formatNumber(row.projects, locale)}
            </BarFigure>
          ),
        },
        {
          key: "open",
          header: t("analytics.openDealValue", {
            currency: baseCurrency ?? "",
          }),
          align: "end",
          render: (row: PhaseRow) =>
            formatMoneyOrAbsent(row.openMinor, baseCurrency, locale),
        },
        {
          key: "won",
          header: t("analytics.wonDealValue", { currency: baseCurrency ?? "" }),
          align: "end",
          render: (row: PhaseRow) =>
            formatMoneyOrAbsent(row.wonMinor, baseCurrency, locale),
        },
      ]}
      rows={phased}
      rowKey={(row) => row.phase}
    />
  );
}

type ProjectListRow = {
  projectId: string;
  name: string;
  phase: string;
  overdue: number;
  open: number;
  derivationUrl: string | null;
};

function projectHref(projectId: string): string {
  // The entity registry owns the route; a hand-written copy goes stale
  // silently, which is the registry's own stated reason to exist.
  return routeHash(ENTITY.project.route(projectId));
}

// A phase spelled to a human, the one way the Projects screen spells it.
function phaseLabel(phase: string, t: ReturnType<typeof useT>): string {
  return isProjectPhase(phase) ? t(PHASE_LABEL[phase]) : phase;
}

// Each project's promises: how many stand open, how many are already late.
export function ProjectCommitmentsTable({
  rows,
  locale,
}: Readonly<{ rows: ReportRow[]; locale: Locale }>) {
  const t = useT();
  const listed: ProjectListRow[] = rows
    .filter((row) => typeof row.project_id === "string")
    .map((row) => ({
      projectId: String(row.project_id),
      name: typeof row.name === "string" ? row.name : "",
      phase: typeof row.phase === "string" ? row.phase : "",
      overdue: rowCount(row, "overdue_commitments"),
      open: rowCount(row, "open_commitments"),
      derivationUrl: rowDerivationUrl(row),
    }));
  if (listed.length === 0) {
    return <EmptyState>{t("analytics.noProjectsYet")}</EmptyState>;
  }
  return (
    <DataTable
      label={t("analytics.reportProjectCommitments")}
      columns={[
        {
          key: "project",
          header: t("analytics.project"),
          render: (row: ProjectListRow) => (
            <CellExplain
              url={row.derivationUrl}
              figure={row.name || t("analytics.project")}
            >
              <a className="link-button" href={projectHref(row.projectId)}>
                {row.name}
              </a>
            </CellExplain>
          ),
        },
        {
          key: "phase",
          header: t("project.phaseLabel"),
          render: (row: ProjectListRow) => phaseLabel(row.phase, t),
        },
        {
          key: "open",
          header: t("analytics.openCommitments"),
          align: "end",
          render: (row: ProjectListRow) => formatNumber(row.open, locale),
        },
        {
          key: "overdue",
          header: t("analytics.overdueCommitments"),
          align: "end",
          render: (row: ProjectListRow) => formatNumber(row.overdue, locale),
        },
      ]}
      rows={listed}
      rowKey={(row) => row.projectId}
    />
  );
}

// Delivering projects nobody has spoken into lately, oldest silence first on
// the server's own ordering.
export function ProjectsGoneQuietTable({
  rows,
  locale,
  timezone,
}: Readonly<{ rows: ReportRow[]; locale: Locale; timezone: string | null }>) {
  const t = useT();
  const listed = rows
    .filter((row) => typeof row.project_id === "string")
    .map((row) => ({
      projectId: String(row.project_id),
      name: typeof row.name === "string" ? row.name : "",
      phase: typeof row.phase === "string" ? row.phase : "",
      quietSince: typeof row.quiet_since === "string" ? row.quiet_since : null,
      derivationUrl: rowDerivationUrl(row),
    }));
  if (listed.length === 0) {
    // Nothing quiet is the good answer, and it should say so rather than
    // draw headers over blank space.
    return <EmptyState>{t("analytics.nothingQuiet")}</EmptyState>;
  }
  return (
    <DataTable
      label={t("analytics.reportProjectsGoneQuiet")}
      columns={[
        {
          key: "project",
          header: t("analytics.project"),
          render: (row: (typeof listed)[number]) => (
            <CellExplain
              url={row.derivationUrl}
              figure={row.name || t("analytics.project")}
            >
              <a className="link-button" href={projectHref(row.projectId)}>
                {row.name}
              </a>
            </CellExplain>
          ),
        },
        {
          key: "phase",
          header: t("project.phaseLabel"),
          render: (row: (typeof listed)[number]) => phaseLabel(row.phase, t),
        },
        {
          key: "quiet",
          header: t("analytics.quietSince"),
          render: (row: (typeof listed)[number]) =>
            row.quietSince && timezone
              ? formatDateTime(row.quietSince, locale, timezone)
              : "—",
        },
      ]}
      rows={listed}
      rowKey={(row) => row.projectId}
    />
  );
}
