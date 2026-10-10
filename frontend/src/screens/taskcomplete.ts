import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import type { useTaskUpdate } from "./taskactions";

// Completing a task from any surface: the confirmation and its Undo are one
// verb, so the Worklist and a record's task list cannot drift apart.
//
// Rejects with the write's error, so each caller says the failure in its own
// place. The toast is raised from this promise, not from per-call callbacks:
// the refetch after the write removes the row and the observer they hang off.
export function useCompleteTask(update: ReturnType<typeof useTaskUpdate>) {
  const t = useT();
  const toast = useToast();
  return (id: string, version: number | undefined) =>
    update
      .mutateAsync({ id, version, body: { is_done: true } })
      .then((completedAt) =>
        toast.show(t("worklist.verb.completed"), {
          action: {
            kind: "undo",
            label: t("worklist.verb.completeUndo"),
            // Pinned on the version the completion produced: the older one
            // would be refused as skew by the write that has just succeeded.
            // A refused undo is said here, since the row is gone by then.
            onAct: () => {
              update
                .mutateAsync({
                  id,
                  version: completedAt,
                  body: { is_done: false },
                })
                .catch(() =>
                  toast.show(t("worklist.verb.completeUndoFailed"), {
                    tone: "danger",
                  }),
                );
            },
          },
        }),
      );
}
