// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { ActionRow } from "../../design-system/actionrow";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { QueryStates } from "../../design-system/querystates";
import { Meter } from "../../design-system/readings";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { UndoConfirm, UndoErrors } from "./import.notices";
import {
  hasProgress,
  ImportProgressPanel,
  panelStalled,
  workerSilent,
} from "./import.progress";
import { PartCounts, ProblemsDownload, partsOf } from "./import.testrun";
import { reasonText } from "./importreason";
import type {
  ImportFormat,
  ImportReport,
  ImportRun,
  ImportRunPart,
} from "./importtypes";
import type { ImportWizardState } from "./importwizard";

type PartProgress = Readonly<{
  part: ImportRunPart;
  done: number;
  finished: boolean;
}>;

// How far the run is into each file. The checkpoint counts rows across the
// files in the order the engine walks them, the format's order.
function progressOf(
  run: ImportRun,
  order: ImportFormat["order"],
): PartProgress[] {
  const parts = [...(run.parts ?? [])].sort(
    (a, b) => order.indexOf(a.object) - order.indexOf(b.object),
  );
  let before = 0;
  return parts.map((part) => {
    const done = Math.min(Math.max(run.checkpoint - before, 0), part.row_count);
    // A file with no rows has nothing to count, so it is finished once the
    // walk has moved past it or the run is complete.
    const finished =
      part.row_count === 0
        ? run.checkpoint > before || run.status === "complete"
        : done >= part.row_count;
    before += part.row_count;
    return { part, done, finished };
  });
}

/** One bar per file of a run, filled from its checkpoint. */
export function PartProgressList({
  run,
  order,
}: Readonly<{ run: ImportRun; order: ImportFormat["order"] }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      {progressOf(run, order).map(({ part, done, finished }) => {
        const label = t(`import.object.${part.object}`);
        let state = t("import.progress.waiting");
        if (finished) {
          state = t("import.progress.done");
        } else if (done > 0) {
          state = t("import.progress.running");
        }
        return (
          <div key={`${part.object}-${part.file}`} className="import__part">
            <div className="import__partHead t-sub">
              <span>{label}</span>
              <span>{state}</span>
            </div>
            <Meter
              value={done}
              max={Math.max(part.row_count, 1)}
              label={t("import.progress.rows", {
                type: label,
                done: formatNumber(done, locale),
                total: formatNumber(part.row_count, locale),
              })}
              flat
            />
          </div>
        );
      })}
    </>
  );
}

/**
 * The import while it runs: one bar per file. A
 * run that stopped says why and resumes from where it stopped.
 */
export function ImportProgress({
  wizard,
  format,
  run,
}: Readonly<{
  wizard: ImportWizardState;
  format: ImportFormat;
  run: ImportRun;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const failed = run.status === "failed";
  return (
    <div className="import">
      {hasProgress(run) ? (
        <ImportProgressPanel run={run} waiting={t("import.progress.working")} />
      ) : (
        <PartProgressList run={run} order={format.order} />
      )}
      {failed ? (
        <>
          <Callout tone="danger" kind="outcome" title={t("import.failedTitle")}>
            {run.error ??
              t("import.failed", {
                checkpoint: formatNumber(run.checkpoint, locale),
              })}
          </Callout>
          <Button
            variant="primary"
            pending={wizard.approve.isPending}
            busyLabel={t("import.importing")}
            onClick={() => wizard.approve.mutate(run)}
          >
            {t("import.resume")}
          </Button>
        </>
      ) : (
        <p className="import__hint t-sub">{t("import.progress.leave")}</p>
      )}
      {wizard.approve.error ? (
        <Callout tone="danger" kind="outcome" title={t("import.commitFailed")}>
          {problemMessageOf(wizard.approve.error, t)}
        </Callout>
      ) : null}
    </div>
  );
}

/**
 * What the run did: the counts per type, the links it made, the rows it left
 * out, and Undo while the run is complete. An undo runs on the server; the
 * step follows it to the end.
 */
export function ImportResult({
  wizard,
  format,
  run,
}: Readonly<{
  wizard: ImportWizardState;
  format: ImportFormat;
  run: ImportRun;
}>) {
  const t = useT();
  return (
    <QueryStates query={wizard.report} pendingLabel={t("common.loading")}>
      {wizard.report.data ? (
        <ResultBody
          wizard={wizard}
          format={format}
          run={run}
          report={wizard.report.data}
        />
      ) : null}
    </QueryStates>
  );
}

function ResultBody({
  wizard,
  format,
  run,
  report,
}: Readonly<{
  wizard: ImportWizardState;
  format: ImportFormat;
  run: ImportRun;
  report: ImportReport;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [asking, setAsking] = useState(false);
  const { undo } = wizard;
  const skipped = report.disposition.skipped;
  return (
    <div className="import">
      {run.status === "complete" ? (
        <Callout tone="success" kind="outcome" title={t("import.done")} />
      ) : null}
      <PartCounts
        parts={partsOf(report, run, format.order)}
        detail={(part) =>
          t("import.result.detail", {
            created: formatNumber(part.disposition.created, locale),
            updated: formatNumber(part.disposition.updated, locale),
          })
        }
      />
      {report.links && report.links.offered > 0 ? (
        <p className="import__hint t-sub">
          {t("import.linksApplied", {
            applied: formatNumber(report.links.applied, locale),
            offered: formatNumber(report.links.offered, locale),
          })}
        </p>
      ) : null}
      {skipped > 0 ? (
        <Callout
          tone="warning"
          kind="standing"
          title={plural("import.result.skipped", skipped, {
            count: formatNumber(skipped, locale),
          })}
        />
      ) : null}
      {run.status === "undoing" ? (
        <UndoProgress
          run={run}
          pending={undo.isPending}
          onContinue={() => undo.mutate(run.id)}
        />
      ) : null}
      {run.status === "undone" ? <UndoOutcome undo={report.undo} /> : null}
      <ActionRow
        className="import__footer"
        primary={
          <>
            <Button variant="ghost" onClick={wizard.reset}>
              {t("import.another")}
            </Button>
            <Button
              variant="primary"
              onClick={() => navigate(settingsHref("import"))}
            >
              {t("import.result.done")}
            </Button>
          </>
        }
      >
        {(report.issues ?? []).length > 0 ? (
          <ProblemsDownload runId={run.id} />
        ) : null}
        {run.status === "complete" ? (
          <Button variant="danger" onClick={() => setAsking(true)}>
            {t("import.result.undo")}
          </Button>
        ) : null}
      </ActionRow>
      {undo.error && !asking ? (
        <Callout tone="danger" kind="outcome" title={t("import.undoFailed")}>
          {problemMessageOf(undo.error, t)}
        </Callout>
      ) : null}
      <UndoConfirm
        open={asking}
        order={format.order}
        pending={undo.isPending}
        error={undo.error ? problemMessageOf(undo.error, t) : null}
        onClose={() => {
          setAsking(false);
          undo.reset();
        }}
        onConfirm={() =>
          undo.mutate(run.id, { onSuccess: () => setAsking(false) })
        }
      />
    </div>
  );
}

// A reversal while it runs: the rows reversed so far, as the import step draws
// its own. Continue undo appears only once the worker has gone quiet, since
// pressing it on a live run could only be refused.
function UndoProgress({
  run,
  pending,
  onContinue,
}: Readonly<{ run: ImportRun; pending: boolean; onContinue: () => void }>) {
  const t = useT();
  const stopped = workerSilent(run);
  const panelSaysStopped = hasProgress(run) && panelStalled(run);
  return (
    <div className="import import--fill">
      {hasProgress(run) ? (
        <ImportProgressPanel run={run} waiting={t("import.undoing")} />
      ) : null}
      {panelSaysStopped ? null : (
        <p className="import__hint t-sub">
          {t(stopped ? "import.result.undoStopped" : "import.result.undoing")}
        </p>
      )}
      {stopped ? (
        <div className="import__actions">
          <Button
            variant="ghost"
            pending={pending}
            busyLabel={t("import.undoing")}
            onClick={onContinue}
          >
            {t("import.continueUndo")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

// What a reversal did: how many rows it reversed, and the rows kept because a
// colleague edited them since (named, never written back over). Also the rows
// it could not reverse, each with why. The report can lack the undo part when its
// read failed, and the run is undone all the same.
function UndoOutcome({ undo }: Readonly<{ undo: ImportReport["undo"] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <div className="import">
      <Callout tone="success" kind="outcome" title={t("import.undone")} />
      {undo ? (
        <>
          <p className="import__hint t-sub">
            {plural("import.undoReversed", undo.reversed_count, {
              rows: formatNumber(undo.reversed_count, locale),
            })}
          </p>
          {undo.kept.length > 0 ? (
            <>
              <p className="import__hint t-sub">{t("import.undoKeptLead")}</p>
              <ul className="import__issues t-sub">
                {undo.kept.map((row) => (
                  <li key={`${row.object}-${row.id}`}>
                    {t(`import.object.${row.object}`)}: {row.id}
                    {row.reason || row.reason_code
                      ? ` — ${reasonText(t, row.reason ?? "", row.reason_code, row.params)}`
                      : null}
                  </li>
                ))}
              </ul>
            </>
          ) : null}
          <UndoErrors rows={undo.errored} />
        </>
      ) : null}
    </div>
  );
}
