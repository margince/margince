// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { WorklistItem } from "./worklist.queries";
import { useNudgeDismissal } from "./worklist.writes";

// The rows a reader could set aside one by one: the same test `rowAnswer`
// applies before it draws the row's own "Not now".
function dismissibleQuietRows(
  rows: readonly WorklistItem[],
): readonly WorklistItem[] {
  return rows.filter(
    (item) =>
      item.source === "relationship_decay" &&
      !item.batch &&
      item.actions.includes("dismiss"),
  );
}

/**
 * "Dismiss all shown" over the quiet relationships drawn beside it. One request
 * sets every listed contact aside for a month, and one Undo puts each back. It
 * names only the rows on screen, so a contact the reader never saw stays. With
 * one row, that row's own verb is enough.
 */
export function QuietDismissAll({
  rows,
}: Readonly<{ rows: readonly WorklistItem[] }>) {
  const t = useT();
  const toast = useToast();
  const { dismissAll, restore } = useNudgeDismissal();
  const contactIds = dismissibleQuietRows(rows).map((item) => item.id);
  if (contactIds.length < 2) {
    return null;
  }
  // mutateAsync and a catch for the Undo, as on the single row. The rows are
  // gone by the time it is pressed, and a per-call callback goes with them.
  const undo = () => {
    Promise.all(
      contactIds.map((contactId) => restore.mutateAsync({ contactId })),
    ).catch(() =>
      toast.show(t("worklist.verb.dismissUndoFailed"), { tone: "danger" }),
    );
  };
  return (
    <Button
      pending={dismissAll.isPending}
      onClick={() =>
        dismissAll.mutate(
          { contactIds },
          {
            onSuccess: () =>
              toast.show(t("worklist.verb.dismissedAll"), {
                action: {
                  kind: "undo",
                  label: t("worklist.verb.dismissUndo"),
                  onAct: undo,
                },
              }),
            onError: () =>
              toast.show(t("worklist.verb.dismissAllFailed"), {
                tone: "danger",
              }),
          },
        )
      }
    >
      {t("worklist.verb.dismissAll")}
    </Button>
  );
}
