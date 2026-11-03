// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Taking one machine change back, from the receipt that reports it.
//
// The receipt used to point at the record's history instead, so disagreeing
// with the machine meant opening the record, finding the entry among its other
// changes and undoing it there — for every one of 150 records. The same restore
// route, with the same If-Match rule, is now one press on the line itself.

import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { magicUndoReasonKey } from "../../lib/magickeys";
import { useReceiptUndo } from "../../lib/receiptundo";
import { isRestoreKind } from "../../lib/recordrestore";

type MagicUndo = components["schemas"]["MagicUndo"];

/**
 * The restore this change can be put back with, or null where it cannot be
 * from here. The button and the line that decides where to say "why not" read
 * this one answer, so the two never disagree about whether a press exists.
 */
export function undoPress(
  undo: MagicUndo | undefined,
  entityType: string,
  entityId: string,
) {
  if (
    !undo?.undoable ||
    !undo.audit_id ||
    undo.version === undefined ||
    !isRestoreKind(entityType)
  ) {
    return null;
  }
  return {
    kind: entityType,
    id: entityId,
    auditId: undo.audit_id,
    version: undo.version,
  };
}

export function MagicUndoButton({
  undo,
  entityType,
  entityId,
}: Readonly<{
  undo: MagicUndo | undefined;
  entityType: string;
  entityId: string;
}>) {
  const t = useT();
  const receiptUndo = useReceiptUndo();
  if (!undo) {
    return null;
  }
  if (receiptUndo.undone) {
    return <span role="status">{t("magic.undo.done")}</span>;
  }
  const press = undoPress(undo, entityType, entityId);
  if (press) {
    return (
      <>
        <Button
          disabled={receiptUndo.pending}
          onClick={() => receiptUndo.press(press)}
        >
          {t("magic.undo.action")}
        </Button>
        {receiptUndo.refused && (
          <p className="t-caption">{receiptUndo.refused}</p>
        )}
      </>
    );
  }
  const reason = magicUndoReasonKey(undo.reason);
  // A reason this build has no words for says nothing rather than printing the
  // server's own token: `no_before_image` is not a sentence a reader can act on.
  return reason ? <span className="t-caption">{t(reason)}</span> : null;
}
