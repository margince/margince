// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { WorklistItem } from "./worklist.queries";
import { useNudgeDismissal } from "./worklist.writes";

/** The quiet-relationship rows on screen that this reader may set aside. */
export function dismissibleQuietRows(
  queue: readonly WorklistItem[],
): readonly WorklistItem[] {
  return queue.filter(
    (item) =>
      item.source === "relationship_decay" &&
      !item.batch &&
      item.actions.includes("dismiss"),
  );
}

// "Dismiss all" over the quiet relationships the reader is looking at: one
// request, all or none, with one Undo that puts each back. It acts on the rows
// shown, so a first mailbox import costs one press per page, not one per row.
export function QuietDismissAll({
  queue,
}: Readonly<{ queue: readonly WorklistItem[] }>) {
  const t = useT();
  const toast = useToast();
  const { dismissAll, restore } = useNudgeDismissal();
  const rows = dismissibleQuietRows(queue);
  if (rows.length < 2) {
    return null;
  }
  const contactIds = rows.map((item) => item.id);
  return (
    <div>
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
                    onAct: () => {
                      Promise.all(
                        contactIds.map((contactId) =>
                          restore.mutateAsync({ contactId }),
                        ),
                      ).catch(() =>
                        toast.show(t("worklist.verb.dismissUndoFailed"), {
                          tone: "danger",
                        }),
                      );
                    },
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
    </div>
  );
}
