// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useLayoutEffect, useRef } from "react";
import { useFocusHandoff } from "../design-system/focushandoff";
import { undoAction, useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";

// On the hook, not on `mutate`: an Undo runs after its row has unmounted, and
// React Query drops a `mutate` call's own callbacks once its observer is gone.
export type MutationOutcome<V> = Readonly<{
  onSuccess: (variables: V) => void;
  onError: (error: Error) => void;
}>;

/**
 * The toasts of a removal that runs at once: what it says, and the Undo that
 * hands its handle to `restore`. A refusal of either is a sticky danger toast;
 * `restoreRefused` words a refused Undo where the server's detail would not.
 */
export function useUndoableRemoval<Handle>(
  words: Readonly<{
    removed: string;
    restored: string;
    restoreRefused?: (error: Error) => string;
  }>,
) {
  const t = useT();
  const toast = useToast();
  const refuse = (message: string) =>
    toast.show(message, { tone: "danger", sticky: true });
  const refused = (error: Error) => refuse(problemMessageOf(error, t));
  return {
    restored: {
      onError: (error: Error) =>
        refuse(words.restoreRefused?.(error) ?? problemMessageOf(error, t)),
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

/** The first live control in the row after `control`'s, else the row before. */
function neighbourVerb(control: Element | null): HTMLElement | null {
  const row = control?.closest(".settingrow");
  const sibling = row?.nextElementSibling ?? row?.previousElementSibling;
  return sibling?.querySelector<HTMLElement>("button:not(:disabled)") ?? null;
}

// A removal unmounts the row holding focus; a neighbour takes it, else `fallback`.
export function useRowHandoff(fallback: () => HTMLElement | null) {
  const verbs = useRef<HTMLSpanElement>(null);
  // Read at unmount, when the ref itself may already be detached.
  const held = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    held.current = verbs.current;
  });
  useFocusHandoff(verbs, () => neighbourVerb(held.current) ?? fallback());
  return verbs;
}
