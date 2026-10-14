// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge, Button, Checkbox, PendingBody } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { QueryStates } from "../design-system/querystates";
import { StatCard } from "../design-system/statcard";
import { StatStrip } from "../design-system/statstrip";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { Locale } from "../i18n/locale";
import { problemMessageOf } from "../lib/problem";
import { ImportContextTagSummary } from "./import.contexttag";
import { ImportRefusal } from "./import.notices";
import { useStopImportTest } from "./importrun";
import type {
  ImportFormat,
  ImportPartReport,
  ImportProblemKind,
  ImportReport,
  ImportRowIssue,
  ImportRun,
} from "./importtypes";
import type { ImportWizardState } from "./importwizard";

/** Where the server builds one run's problems as a spreadsheet. */
export function problemsHref(id: string): string {
  return `/v1/imports/${encodeURIComponent(id)}/problems.csv`;
}

const PROBLEM_TONE = {
  broken_link: "warning",
  dropped_link: "default",
  invalid_value: "danger",
  unknown_owner: "info",
  new_tag: "info",
  duplicate: "info",
  permission: "danger",
  unusable_row: "danger",
} as const satisfies Record<ImportProblemKind, string>;

// The report's parts in the format's order. An older report of one CSV has no
// parts, so its whole disposition is the one part.
export function partsOf(
  report: ImportReport,
  run: ImportRun | undefined,
  order: ImportFormat["order"],
): ImportPartReport[] {
  const parts = report.parts?.length
    ? report.parts
    : [
        {
          file: run?.file_name ?? "",
          object: run?.object ?? order[0] ?? "company",
          rows_read: report.rows_read,
          disposition: report.disposition,
        },
      ];
  return [...parts].sort(
    (a, b) => order.indexOf(a.object) - order.indexOf(b.object),
  );
}

function countOf(
  issues: readonly ImportRowIssue[],
  ...kinds: ImportProblemKind[]
): number {
  return issues.filter((issue) => issue.kind && kinds.includes(issue.kind))
    .length;
}

/**
 * What the run will do, before anything is written. It shows the counts per
 * type and every problem with where it is, and offers to bring rows with
 * broken links in without the link. Import approves this report and no other.
 */
export function TestRunStep({
  wizard,
  format,
}: Readonly<{ wizard: ImportWizardState; format: ImportFormat }>) {
  const t = useT();
  const { report, runId } = wizard;
  if (wizard.run.data?.status === "validating") {
    return <TestingBody runId={wizard.run.data.id} />;
  }
  return (
    <QueryStates query={report} pendingLabel={t("common.loading")}>
      {report.data && runId ? (
        <TestRunBody
          wizard={wizard}
          format={format}
          report={report.data}
          runId={runId}
        />
      ) : null}
    </QueryStates>
  );
}

// A test can read for minutes, so the one waiting on it can stop it.
function TestingBody({ runId }: Readonly<{ runId: string }>) {
  const t = useT();
  const stop = useStopImportTest();
  return (
    <div className="import">
      <PendingBody visible label={t("import.test.testing")} />
      <div className="import__actions">
        <Button pending={stop.isPending} onClick={() => stop.mutate(runId)}>
          {t("import.test.stop")}
        </Button>
      </div>
      {stop.error ? (
        <ImportRefusal title={t("import.test.stopFailed")} error={stop.error} />
      ) : null}
    </div>
  );
}

function TestRunBody({
  wizard,
  format,
  report,
  runId,
}: Readonly<{
  wizard: ImportWizardState;
  format: ImportFormat;
  report: ImportReport;
  runId: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const invalid = countOf(report.issues, "invalid_value");
  const linkless = countOf(report.issues, "broken_link", "dropped_link");
  const restaging = wizard.test.isPending;
  const landing = report.disposition.created + report.disposition.updated;
  return (
    <div className="import">
      <PartCounts
        parts={partsOf(report, wizard.run.data, format.order)}
        detail={(part) =>
          t("import.test.detail", {
            created: formatNumber(part.disposition.created, locale),
            updated: formatNumber(part.disposition.updated, locale),
            unchanged: formatNumber(part.disposition.unchanged, locale),
          })
        }
      />
      <ImportContextTagSummary
        tagID={wizard.contextTagID || (wizard.run.data?.context_tag_id ?? "")}
      />
      {invalid > 0 ? (
        <Callout
          tone="warning"
          kind="standing"
          title={plural("import.test.invalid", invalid, {
            count: formatNumber(invalid, locale),
          })}
        >
          {t("import.test.invalidBody")}
        </Callout>
      ) : null}
      <ProblemTable issues={report.issues} />
      {linkless > 0 ? (
        <Checkbox
          checked={wizard.anyway}
          disabled={restaging}
          onChange={(event) => wizard.setAnyway(event.target.checked)}
          label={plural("import.test.anyway", linkless, {
            count: formatNumber(linkless, locale),
          })}
        />
      ) : null}
      <div className="import__actions">
        {report.issues.length > 0 ? (
          <a className="link-button" href={problemsHref(runId)} download>
            {t("import.result.problemsFile")}
          </a>
        ) : null}
        <Button variant="ghost" onClick={wizard.reset}>
          {t("import.test.reupload")}
        </Button>
        <Button
          variant="primary"
          disabled={restaging}
          pending={wizard.approve.isPending}
          busyLabel={t("import.importing")}
          onClick={() => {
            const run = wizard.run.data;
            if (run) wizard.approve.mutate(run);
          }}
        >
          {wizard.anyway
            ? t("import.test.importAnyway")
            : plural("import.test.import", landing, {
                count: formatNumber(landing, locale),
              })}
        </Button>
      </div>
      {wizard.approve.error ? (
        <Callout tone="danger" kind="outcome" title={t("import.commitFailed")}>
          {problemMessageOf(wizard.approve.error, t)}
        </Callout>
      ) : null}
      {wizard.test.error ? (
        <ImportRefusal
          title={t("import.map.stageFailed")}
          error={wizard.test.error}
        />
      ) : null}
    </div>
  );
}

/** One reading per file: the rows it writes, and how they split. */
export function PartCounts({
  parts,
  detail,
}: Readonly<{
  parts: readonly ImportPartReport[];
  detail: (part: ImportPartReport) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <StatStrip className="import__counts" label={t("import.test.counts")}>
      {parts.map((part) => (
        <StatCard
          key={`${part.object}-${part.file}`}
          label={t(`import.object.${part.object}`)}
          value={formatNumber(
            part.disposition.created + part.disposition.updated,
            locale,
          )}
          detail={detail(part)}
        />
      ))}
    </StatStrip>
  );
}

/** Where a problem is: its row and column, or the whole file when it has no row. */
function issueWhere(
  issue: ImportRowIssue,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  if (issue.line === undefined) return t("import.test.wholeFile");
  const line = formatNumber(issue.line, locale);
  return issue.column
    ? t("import.test.rowColumn", { line, column: issue.column })
    : t("import.test.row", { line });
}

/** Every problem the report names, with the file, row and column it is in. */
export function ProblemTable({
  issues,
}: Readonly<{ issues: readonly ImportRowIssue[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (issues.length === 0) return null;
  return (
    <DataTable<ImportRowIssue>
      label={plural("import.test.problems", issues.length, {
        count: formatNumber(issues.length, locale),
      })}
      rows={[...issues]}
      rowKey={(issue) =>
        `${issue.file ?? ""}-${issue.line ?? ""}-${issue.column ?? ""}-${issue.kind ?? ""}-${issue.value ?? ""}`
      }
      columns={[
        {
          key: "where",
          header: t("import.test.col.where"),
          render: (issue) => (
            <>
              {issue.file ? <code>{issue.file}</code> : null}
              <div className="t-caption">{issueWhere(issue, t, locale)}</div>
            </>
          ),
        },
        {
          key: "problem",
          header: t("import.test.col.problem"),
          grow: true,
          render: (issue) => (
            <>
              {issue.kind ? (
                <Badge tone={PROBLEM_TONE[issue.kind]}>
                  {t(`import.problem.${issue.kind}`)}
                </Badge>
              ) : null}
              <div className="t-sub">{issue.reason}</div>
            </>
          ),
        },
        {
          key: "value",
          header: t("import.test.col.value"),
          render: (issue) => (issue.value ? <code>{issue.value}</code> : null),
        },
      ]}
    />
  );
}
