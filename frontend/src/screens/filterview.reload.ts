// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryObserverResult } from "@tanstack/react-query";
import { useState } from "react";
import { problemCodeOf } from "./common";
import { openableTree, type SavedView } from "./savedviews.queries";
import type { Node } from "./segmentpredicate";

export type Reread = () => Promise<QueryObserverResult<SavedView>>;

/**
 * "Reload view": read the view again and start over from what it says now, or
 * learn that it is gone. Its own answer, so a refetch nobody asked for moves
 * nothing on the page.
 */
export function useReload(
  reread: Reread,
  onFresh: (view: SavedView, tree: Node) => void,
) {
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<unknown>(null);
  const [lost, setLost] = useState(false);
  const reload = () => {
    setPending(true);
    setFailure(null);
    void reread().then((answer) => {
      setPending(false);
      if (answer.isError && problemCodeOf(answer.error) !== "not_found") {
        setFailure(answer.error);
        return;
      }
      const view = answer.isError ? undefined : answer.data;
      const fresh = view ? openableTree(view) : null;
      if (!view || fresh === null) {
        setLost(true);
        return;
      }
      onFresh(view, fresh);
    });
  };
  return { reload, pending, failure, lost, clear: () => setFailure(null) };
}
