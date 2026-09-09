// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { ImportReport } from "./importtypes";

// What an UNDO says about itself: that it stopped part-way, and which rows it
// could not put back.
//
// Their own file because both are bands whose body is a LIST or a caveat about
// the run rather than about the file, and the wizard's own function already
// carries the upload, the mapping, the dry run, the commit and the reversal.

type UndoOutcome = NonNullable<ImportReport["undo"]>;

/**
 * The reversal stopped part-way, so continuing picks up where it stopped.
 *
 * An `event`: it reports a run that halted rather than a press the reader has
 * just made, and it is mentioned rather than allowed to interrupt.
 */
export function UndoInterruptedNotice({
  interrupted,
}: Readonly<{ interrupted: boolean }>) {
  const t = useT();
  if (!interrupted) {
    return null;
  }
  return (
    <Callout tone="warn" kind="event" title={t("import.undoInterruptedTitle")}>
      {t("import.undoInterrupted")}
    </Callout>
  );
}

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
    <Callout tone="warn" kind="outcome" title={t("import.undoErroredLead")}>
      <ul className="import__issues t-sub">
        {rows.map((row) => (
          <li key={`${row.object}-${row.id}`}>
            {t(`import.object.${row.object}`)} — {row.id}: {row.reason}
          </li>
        ))}
      </ul>
    </Callout>
  );
}
