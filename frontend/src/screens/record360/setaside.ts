// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useT } from "../../i18n";
import { useUndoableRemoval } from "../../lib/undoableremoval";

/**
 * The "Not now" every record page's move carries, and the Undo after it.
 *
 * The deal, contact and company cards each store the set-aside in their own
 * place, but the reader meets one verb. The row leaves once the server's
 * re-read says so, and a toast offers the way back. Each page hands in its own
 * two calls and the read to refresh.
 *
 * `idOf` names an item, so only the row in flight is held while it saves.
 */
export function useSetAside<Item>({
  setAside,
  restore,
  refresh,
  idOf,
}: Readonly<{
  setAside: (item: Item) => Promise<void>;
  restore: (item: Item) => Promise<void>;
  refresh: () => Promise<unknown>;
  idOf: (item: Item) => string;
}>) {
  const t = useT();
  const toasts = useUndoableRemoval<Item>({
    removed: t("record.setAside.done"),
    restored: t("record.setAside.restored"),
  });
  // The callbacks live on the hooks rather than on `mutate`. The Undo runs
  // after the row has left, and React Query drops a call's own callbacks once
  // its component is gone.
  const back = useMutation({
    mutationFn: async (item: Item) => {
      await restore(item);
      await refresh();
    },
    onSuccess: toasts.restored.onSuccess,
    onError: toasts.restored.onError,
  });
  const removed = toasts.removed((item) => back.mutate(item));
  const away = useMutation({
    mutationFn: async (item: Item) => {
      await setAside(item);
      await refresh();
    },
    onSuccess: (_data, item) => removed.onSuccess(item),
  });
  return {
    deferOf: (item: Item) => ({
      onDefer: () => away.mutate(item),
      pending:
        away.isPending &&
        away.variables !== undefined &&
        idOf(away.variables) === idOf(item),
    }),
    // Said beside the row rather than in a toast. The row staying put is what
    // the reader looks at, and a word there says the click did not miss.
    failure: away.error,
  };
}
