// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One Undo press from a receipt, and the one refresh after it.
//
// After a success and after a refusal, the receipt and its record are read
// again. A refusal usually means the record moved. An open editor keeps its
// own copy, so its If-Match still catches the change.

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useT } from "../i18n";
import { isEntityKind } from "./entity";
import { undoRefusalKey, VERSION_SKEW_CODE } from "./historyundo";
import { problemCodeOf, problemMessageOf } from "./problem";
import { type RestorePress, useRecordRestore } from "./recordrestore";
import { invalidateRecord } from "./recordwritekeys";
import { invalidateHandledChange } from "./workkeys";

function refreshAfterUndo(
  client: ReturnType<typeof useQueryClient>,
  press: RestorePress,
): Promise<unknown> {
  return Promise.all([
    invalidateHandledChange(client),
    client.invalidateQueries({
      queryKey: ["record-history", press.kind, press.id],
    }),
    client.invalidateQueries({
      queryKey: ["field-history", press.kind, press.id],
    }),
    isEntityKind(press.kind)
      ? invalidateRecord(client, press.kind, press.id)
      : undefined,
  ]);
}

/** Presses Undo on one receipt line and reports what the server answered. */
export function useReceiptUndo() {
  const t = useT();
  const client = useQueryClient();
  const [undone, setUndone] = useState(false);
  const [refused, setRefused] = useState<string | null>(null);
  const restore = useRecordRestore();
  const press = (variables: RestorePress) =>
    restore.mutate(variables, {
      onSuccess: () => {
        setRefused(null);
        setUndone(true);
        void refreshAfterUndo(client, variables);
      },
      onError: (error) => {
        const code = problemCodeOf(error);
        const key =
          code === VERSION_SKEW_CODE
            ? "history.undo.versionSkew"
            : undoRefusalKey(code);
        setRefused(key ? t(key) : problemMessageOf(error, t));
        void refreshAfterUndo(client, variables);
      },
    });
  return { press, undone, refused, pending: restore.isPending };
}
