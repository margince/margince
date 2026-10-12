// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { undoAction, useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";

// On the hook, not on `mutate`: an Undo runs after its row has unmounted, and
// React Query drops a `mutate` call's own callbacks once its observer is gone.
export type MutationOutcome<V> = Readonly<{
  onSuccess: (variables: V) => void;
  onError: (error: Error) => void;
}>;

/**
 * The toasts of a removal that runs at once: what it says, the Undo that hands
 * its handle to `restore`, and a refusal of either as a sticky danger toast.
 */
export function useUndoableRemoval<Handle>(
  words: Readonly<{ removed: string; restored: string }>,
) {
  const t = useT();
  const toast = useToast();
  const refused = (error: Error) =>
    toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
  return {
    restored: {
      onError: refused,
      onSuccess: () => toast.show(words.restored),
    } satisfies MutationOutcome<Handle>,
    removed: (
      restore: (handle: Handle) => void,
      undoable = true,
    ): MutationOutcome<Handle | null> => ({
      onError: refused,
      onSuccess: (handle) =>
        toast.show(
          words.removed,
          handle && undoable
            ? { action: undoAction(t("common.undo"), () => restore(handle)) }
            : undefined,
        ),
    }),
  };
}
