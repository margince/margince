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

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { isEntityKind } from "../app/entity";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { problemCodeOf, problemMessageOf } from "./common";
import { VERSION_SKEW_CODE } from "./historyundo";
import { useRecordRestore } from "./recordrestore";
import { type Receipt, worklistKey } from "./worklist.queries";

export function ReceiptUndo({ receipt }: Readonly<{ receipt: Receipt }>) {
  const t = useT();
  const client = useQueryClient();
  // What the server said when it refused the press. A greyed control with no
  // sentence tells the reader nothing about why their undo did not happen.
  const [refused, setRefused] = useState<string | null>(null);

  const putBack = useRecordRestore({
    onSuccess: () => {
      setRefused(null);
      // The whole worklist, not the receipt alone: putting a close date back
      // changes the deal, and the lanes above this panel are read off the same
      // deals. Refetching only the receipt would leave the page agreeing with
      // itself about a date that had just moved.
      client.invalidateQueries({ queryKey: worklistKey });
    },
    onError: (error) => {
      const code = problemCodeOf(error);
      if (code === VERSION_SKEW_CODE) {
        // The deal moved under the receipt rather than the change being
        // unrestorable. Re-read, so the reader decides against what the record
        // says now.
        client.invalidateQueries({ queryKey: worklistKey });
        setRefused(t("history.undo.versionSkew"));
        return;
      }
      setRefused(problemMessageOf(error, t));
    },
  });

  const undo = receipt.undo;
  // No undo on the receipt means there is nothing to offer: an approval the
  // system decided is revisitable through the record it named.
  if (!undo) {
    return null;
  }
  if (undo.reversed) {
    return (
      <span className="t-caption">{t("worklist.handled.putBackDone")}</span>
    );
  }
  // Every correction this panel shows is a deal's close date. The subject is
  // what says WHICH deal, and a receipt that lost it has no record to restore
  // against — the control is not offered rather than guessing at one.
  //
  // The kind is CHECKED rather than cast. The wire's subject union is wider
  // than the records this app has screens for — `activity` is on it and is the
  // timeline rather than a 360 record — and the restore route is addressed by
  // record. A subject naming something else gets no control instead of a
  // request built on a path that does not exist.
  const subject = receipt.subject;
  if (!subject || !isEntityKind(subject.type)) {
    return null;
  }
  const kind = subject.type;
  return (
    <>
      <Button
        small
        variant="ghost"
        pending={putBack.isPending}
        busyLabel={t("history.undo.busy")}
        onClick={() =>
          putBack.mutate({
            kind,
            id: subject.id,
            auditId: undo.audit_log_id,
            version: undo.version,
          })
        }
      >
        {t("history.undo.action")}
      </Button>
      {refused && <p className="t-caption">{refused}</p>}
    </>
  );
}
