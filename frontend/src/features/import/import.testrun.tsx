// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ActionRow } from "../../design-system/actionrow";
import { Button, Checkbox, PendingBody } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { QueryStates } from "../../design-system/querystates";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import { ImportContextTagSummary } from "./import.contexttag";
import { ImportRefusal } from "./import.notices";
import { ProblemsDownload, ProblemTable } from "./import.problems";
import { hasProgress, ImportProgressPanel } from "./import.progress";
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

/** The issue list, tolerant of a report that sends none. */
function issuesOf(report: ImportReport): readonly ImportRowIssue[] {
  return report.issues ?? [];
}

// The rows with an issue of these kinds. A row with several cells at fault has
// an issue for each, and still counts once.
function countOf(
  issues: readonly ImportRowIssue[],
  ...kinds: ImportProblemKind[]
): number {
  const rows = new Set<string>();
  issues.forEach((issue, index) => {
    if (!issue.kind || !kinds.includes(issue.kind)) return;
    rows.add(
      issue.line === undefined
        ? `issue ${index}`
        : `${issue.file ?? ""}\u0000${issue.line}`,
    );
  });
  return rows.size;
}

// How many deals the parts say land already closed, by the part field named.
function sumOf(
  report: ImportReport,
  field: "closed_won" | "closed_lost",
): number {
  return (report.parts ?? []).reduce(
    (sum, part) => sum + (part[field] ?? 0),
    0,
  );
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
    return <TestingBody run={wizard.run.data} />;
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
function TestingBody({ run }: Readonly<{ run: ImportRun }>) {
  const t = useT();
  const stop = useStopImportTest();
  return (
    <div className="import">
      {hasProgress(run) ? (
        <ImportProgressPanel run={run} waiting={t("import.test.testing")} />
      ) : (
        <PendingBody visible label={t("import.test.testing")} />
      )}
      <div className="import__actions">
        <Button pending={stop.isPending} onClick={() => stop.mutate(run.id)}>
          {t("import.test.stop")}
        </Button>
      </div>
      {stop.error ? (
        <ImportRefusal title={t("import.test.stopFailed")} error={stop.error} />
      ) : null}
    </div>
  );
}

function TestRunNotices({ report }: Readonly<{ report: ImportReport }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const invalid = countOf(issuesOf(report), "invalid_value");
  const closedWon = sumOf(report, "closed_won");
  const closedLost = sumOf(report, "closed_lost");
  const closed = closedWon + closedLost;
  return (
    <>
      {closed > 0 ? (
        <Callout
          tone="info"
          kind="standing"
          title={plural("import.test.closedDeals", closed, {
            count: formatNumber(closed, locale),
          })}
        >
          {t("import.test.closedDealsBody", {
            won: formatNumber(closedWon, locale),
            lost: formatNumber(closedLost, locale),
          })}
        </Callout>
      ) : null}
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
    </>
  );
}

function PrimaryAction({
  wizard,
  nothingToWrite,
  landing,
}: Readonly<{
  wizard: ImportWizardState;
  nothingToWrite: boolean;
  landing: number;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (nothingToWrite) {
    return (
      <Button
        variant="primary"
        pending={wizard.cancelling.isPending}
        onClick={wizard.cancel}
      >
        {t("import.test.nothing.done")}
      </Button>
    );
  }
  return (
    <Button
      variant="primary"
      disabled={wizard.test.isPending || landing === 0}
      pending={wizard.approve.isPending}
      busyLabel={t("import.importing")}
      onClick={() => {
        const run = wizard.run.data;
        if (run) wizard.approve.mutate(run);
      }}
    >
      {plural(
        wizard.anyway ? "import.test.importAnyway" : "import.test.import",
        landing,
        { count: formatNumber(landing, locale) },
      )}
    </Button>
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
  const linkless = countOf(issuesOf(report), "broken_link", "dropped_link");
  const restaging = wizard.test.isPending;
  const landing = report.disposition.created + report.disposition.updated;
  const empty = report.rows_read === 0;
  // Rows with a broken link can still land, but only once the box is ticked.
  const nothingToWrite =
    empty || (landing === 0 && (linkless === 0 || !wizard.anyway));
  const skippedRows = report.disposition.skipped > 0;
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
      <TestRunNotices report={report} />
      <ProblemTable issues={issuesOf(report)} />
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
      {nothingToWrite ? (
        <Callout
          tone="info"
          kind="standing"
          title={t("import.test.nothing.title")}
        >
          {empty
            ? t("import.test.nothing.empty")
            : t("import.test.nothing.body")}
          {skippedRows ? ` ${t("import.test.nothing.skipped")}` : null}
        </Callout>
      ) : null}
      <ActionRow
        className="import__footer"
        primary={
          <>
            <Button variant="ghost" onClick={wizard.reupload}>
              {t("import.test.reupload")}
            </Button>
            <PrimaryAction
              wizard={wizard}
              nothingToWrite={nothingToWrite}
              landing={landing}
            />
          </>
        }
      >
        {issuesOf(report).length > 0 ? (
          <ProblemsDownload runId={runId} />
        ) : null}
      </ActionRow>
      {wizard.approve.error ? (
        <Callout tone="danger" kind="outcome" title={t("import.commitFailed")}>
          {problemMessageOf(wizard.approve.error, t)}
        </Callout>
      ) : null}
      {wizard.test.error ? (
        <ImportRefusal
          title={t("import.map.stageFailed")}
          error={wizard.test.error}
          format={format}
          keepsDraft
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
    <StatStrip
      className="import__counts"
      label={t("import.test.counts")}
      density="compact"
    >
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
