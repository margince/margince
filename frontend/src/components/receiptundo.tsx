// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The way back from a change nobody was asked about.
//
// Every other row in the handled panel reports a decision somebody made, and
// carries no verb because the work is done. A correction the nightly sweep
// applied is different in the one way that matters: the reader never agreed to
// it. So this row, alone on this surface, offers the reader the way back — and
// it is the only telling they get, because no approval card was ever staged.
//
// A correction already put back keeps its row and says so rather than
// disappearing. A row that vanished on success would leave the reader unsure
// whether their press landed or the list simply moved.

import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import type { Receipt } from "../lib/handled";
import { useReceiptUndo } from "../lib/receiptundo";

export function ReceiptUndo({ receipt }: Readonly<{ receipt: Receipt }>) {
  const t = useT();
  const writable = useCanWrite("deal", "update");
  const receiptUndo = useReceiptUndo();

  const undo = receipt.undo;
  // No undo on the receipt means there is nothing to offer: an approval the
  // system decided is revisitable through the record it named.
  if (!undo) {
    return null;
  }
  if (undo.reversed || receiptUndo.undone) {
    return <span>{t("worklist.handled.putBackDone")}</span>;
  }
  // This receipt restores a deal correction; other subject kinds have no
  // correction engine and cannot borrow its permission or restore route.
  if (!writable) return null;
  const subject = receipt.subject;
  if (subject?.type !== "deal") {
    return null;
  }
  const kind = subject.type;
  return (
    <>
      <Button
        variant="ghost"
        pending={receiptUndo.pending}
        busyLabel={t("history.undo.busy")}
        onClick={() =>
          receiptUndo.press({
            kind,
            id: subject.id,
            auditId: undo.audit_log_id,
            version: undo.version,
          })
        }
      >
        {t("common.undo")}
      </Button>
      {receiptUndo.refused && <p>{receiptUndo.refused}</p>}
    </>
  );
}
