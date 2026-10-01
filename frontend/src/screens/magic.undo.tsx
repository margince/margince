// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Taking one machine change back, from the receipt that reports it.
//
// The receipt used to point at the record's history instead, so disagreeing
// with the machine meant opening the record, finding the entry among its other
// changes and undoing it there — for every one of 150 records. The same restore
// route, with the same If-Match rule, is now one press on the line itself.

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../api/schema";
import { isEntityKind } from "../app/entity";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import { magicUndoReasonKey } from "./magic.keys";
import { magicKey } from "./magic.queries";
import { useRecordRestore } from "./recordrestore";

type MagicUndo = components["schemas"]["MagicUndo"];

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
  const client = useQueryClient();
  const [undone, setUndone] = useState(false);
  const [refused, setRefused] = useState<string | null>(null);
  const restore = useRecordRestore({
    onSuccess: () => {
      setRefused(null);
      setUndone(true);
      void client.invalidateQueries({ queryKey: magicKey });
    },
    onError: (error) => setRefused(problemMessageOf(error, t)),
  });
  if (!undo) {
    return null;
  }
  if (undone) {
    return <span role="status">{t("magic.undo.done")}</span>;
  }
  if (
    undo.undoable &&
    undo.audit_id &&
    undo.version !== undefined &&
    isEntityKind(entityType)
  ) {
    const press = {
      kind: entityType,
      id: entityId,
      auditId: undo.audit_id,
      version: undo.version,
    };
    return (
      <>
        <Button
          disabled={restore.isPending}
          onClick={() => restore.mutate(press)}
        >
          {t("magic.undo.action")}
        </Button>
        {refused && <p className="t-caption">{refused}</p>}
      </>
    );
  }
  const reason = magicUndoReasonKey(undo.reason);
  // A reason this build has no words for says nothing rather than printing the
  // server's own token: `no_before_image` is not a sentence a reader can act on.
  return reason ? <span className="t-caption">{t(reason)}</span> : null;
}
