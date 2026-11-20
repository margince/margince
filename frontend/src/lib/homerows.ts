// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The reader's own action on a row Home lists. Home leaves the row at once and
// shows the Updating mark. Once the write answered, the server's day is read
// again. A failed write so brings the row back from the server, not a kept copy.

import type { InfiniteData, QueryClient } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { homeDayOptions } from "./worklistday";

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

/** A row by its identity: the lanes mint ids independently, so both parts count. */
export type HomeRow = Readonly<{ source: string; id: string }>;

function without(rows: readonly HomeRow[]) {
  const leaves = (item: WorklistItem) =>
    rows.some((row) => row.id === item.id && row.source === item.source);
  return (page: Worklist): Worklist => ({
    ...page,
    queue: page.queue.filter((item) => !leaves(item)),
    ...(page.focus
      ? {
          focus: {
            ...page.focus,
            items: page.focus.items.filter((i) => !leaves(i)),
          },
        }
      : {}),
  });
}

/**
 * Takes the rows out of the cached Home day and marks it as updating. Returns
 * the call that reads the day again; make it when the write answered, whether it
 * succeeded or not.
 *
 * Only a query found before the wait is written, and only while the cache
 * still holds it. A sign-out in between so leaves nothing behind.
 */
export async function hideHomeRows(
  client: QueryClient,
  rows: readonly HomeRow[],
): Promise<() => void> {
  const { queryKey } = homeDayOptions();
  const cache = client.getQueryCache();
  const found = cache.find<InfiniteData<Worklist>>({ queryKey, exact: true });
  await client.cancelQueries({ queryKey, exact: true });
  const data = found?.state.data;
  if (found && data && cache.get(found.queryHash) === found) {
    const drop = without(rows);
    client.setQueryData<InfiniteData<Worklist>>(queryKey, {
      ...data,
      pages: data.pages.map((page, at) =>
        at === 0 ? { ...drop(page), served_stale: true } : drop(page),
      ),
    });
  }
  return () => {
    void client.invalidateQueries({ queryKey, exact: true });
  };
}
