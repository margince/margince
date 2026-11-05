// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useT } from "../../i18n";
import {
  ProblemError,
  problemCode,
  problemFieldErrorsOf,
  problemMessageOf,
} from "../../lib/problem";
import { isRecord } from "../../lib/problembody";
import { runHref } from "./importaddress";
import { reasonText } from "./importreason";
import type { ImportObject, ImportReport } from "./importtypes";

// What an UNDO says about itself: that it stopped part-way, and which rows it
// could not put back.
//
// Their own file because both are bands whose body is a LIST or a caveat about
// the run rather than about the file, and the wizard's own function already
// carries the upload, the mapping, the dry run, the commit and the reversal.

type UndoOutcome = NonNullable<ImportReport["undo"]>;

/**
 * The rows a reversal could not put back, named with why — under ONE heading,
 * as the notice's own body.
 *
 * The lead used to be a band standing OVER the list it introduced, which is a
 * heading in notice clothing: the rows are what the reader came here to read,
 * and a callout whose body is the list says the same thing with one anatomy.
 */
export function UndoErrors({
  rows,
}: Readonly<{ rows: UndoOutcome["errored"] }>) {
  const t = useT();
  if (rows.length === 0) {
    return null;
  }
  return (
    <Callout tone="warning" kind="outcome" title={t("import.undoErroredLead")}>
      <ul className="import__issues t-sub">
        {rows.map((row) => (
          <li key={`${row.object}-${row.id}`}>
            {t(`import.object.${row.object}`)} — {row.id}:{" "}
            {reasonText(t, row.reason, row.reason_code, undefined)}
          </li>
        ))}
      </ul>
    </Callout>
  );
}

/**
 * What an undo will do, asked before it starts. `order` is the import order
 * from the format; the undo walks it backwards, so the list is reversed here.
 */
export function UndoConfirm({
  open,
  order,
  pending,
  error,
  onClose,
  onConfirm,
}: Readonly<{
  open: boolean;
  order: readonly ImportObject[];
  pending: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: () => void;
}>) {
  const t = useT();
  const reversed = [...order]
    .reverse()
    .map((object) => t(`import.object.${object}`));
  return (
    <ConfirmModal
      open={open}
      title={t("import.undoConfirm.title")}
      confirmLabel={t("import.undoConfirm.confirm")}
      confirmVariant="danger"
      pending={pending}
      error={error}
      onClose={onClose}
      onConfirm={onConfirm}
    >
      {reversed.length > 0 ? (
        <p>{t("import.undoConfirm.order", { order: reversed.join(", ") })}</p>
      ) : null}
      <ul className="import__issues">
        <li>{t("import.undoConfirm.archived")}</li>
        <li>{t("import.undoConfirm.kept")}</li>
        <li>{t("import.undoConfirm.updated")}</li>
        <li>{t("import.undoConfirm.tags")}</li>
      </ul>
    </ConfirmModal>
  );
}

// The run that holds a busy source name, when the reader may read it.
function busyRunOf(error: unknown): string | null {
  if (!(error instanceof ProblemError)) return null;
  const { problem } = error;
  if (problemCode(problem) !== "import_source_busy" || !isRecord(problem)) {
    return null;
  }
  const runId = isRecord(problem.details) ? problem.details.run_id : null;
  return typeof runId === "string" ? runId : null;
}

/**
 * Why the server refused an upload or a test run. A file too long for one
 * test run is named as such and says to split it. A busy source name links to
 * the run holding it.
 */
export function ImportRefusal({
  title,
  error,
  keepsDraft = false,
}: Readonly<{
  title: string;
  error: unknown;
  /** The refused call was a stage, which leaves the upload as a draft. Undo,
   * cancel and approve leave none, so the draft sentence is theirs to omit. */
  keepsDraft?: boolean;
}>) {
  const t = useT();
  const tooLong = problemFieldErrorsOf(error).some(
    (field) => field.code === "too_many_rows",
  );
  const busyRun = busyRunOf(error);
  return (
    <Callout
      tone="danger"
      kind="outcome"
      title={tooLong ? t("import.upload.tooManyRowsTitle") : title}
      actions={
        busyRun ? (
          <AppLink className="link-button" href={runHref(busyRun)}>
            {t("import.sourceBusyOpen")}
          </AppLink>
        ) : undefined
      }
    >
      {tooLong ? t("import.upload.tooManyRows") : problemMessageOf(error, t)}
      {busyRun && keepsDraft ? <p>{t("import.sourceBusyDraft")}</p> : null}
    </Callout>
  );
}
