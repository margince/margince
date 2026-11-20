// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The reader's own action on a row Home lists. Home leaves the row at once and
// shows the Updating mark; the day read after the write brings the fresh day.

import type { InfiniteData, QueryClient } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { isHomeDayRead, worklistKey } from "./workkeys";

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

/** A row by the id the lanes give it. Without a source, any lane's id matches. */
export type HomeRow = Readonly<{ id: string; source?: string }>;

/** What an action does with the hidden rows once its write answered. */
export type HomeHide = Readonly<{
  /** The write succeeded: the next fresh day may end the hiding. */
  settle: () => void;
  /** The write failed: the rows come back. */
  restore: () => void;
}>;

type Hidden = HomeRow & { settled: boolean; expiresAt: number };

// A kept day that still lists the row can answer before the fresh one. These
// rows stay out of every answer until a fresh first page arrives after the write.
const HIDE_LIMIT_MS = 2 * 60_000;
let hidden: Hidden[] = [];

const isHidden = (item: WorklistItem) =>
  hidden.some(
    (row) =>
      row.id === item.id &&
      (row.source === undefined || row.source === item.source),
  );

function withoutHidden(page: Worklist): Worklist {
  const focus = page.focus
    ? { ...page.focus, items: page.focus.items.filter((i) => !isHidden(i)) }
    : undefined;
  return {
    ...page,
    queue: page.queue.filter((item) => !isHidden(item)),
    ...(focus ? { focus } : {}),
  };
}

/** A page just read, without the rows the reader has acted on since. */
export function withoutActedOnRows(
  page: Worklist,
  isFirstPage: boolean,
  now: number = Date.now(),
): Worklist {
  hidden = hidden.filter((row) => row.expiresAt > now);
  if (isFirstPage && !page.served_stale) {
    hidden = hidden.filter((row) => !row.settled);
  }
  return hidden.length === 0 ? page : withoutHidden(page);
}

function isDay(data: unknown): data is InfiniteData<Worklist> {
  return (
    typeof data === "object" &&
    data !== null &&
    "pages" in data &&
    Array.isArray(data.pages)
  );
}

function marked(data: InfiniteData<Worklist>): InfiniteData<Worklist> {
  const pages = data.pages.map((page, at) =>
    at === 0
      ? { ...withoutHidden(page), served_stale: true }
      : withoutHidden(page),
  );
  return { ...data, pages };
}

const homeDays = {
  queryKey: worklistKey,
  predicate: (query: { queryKey: readonly unknown[] }) =>
    isHomeDayRead(query.queryKey),
};

/**
 * Takes the rows out of every cached Home day and marks the day as updating.
 * The caller refetches once the write answered (invalidating worklistKey), and
 * calls settle on success or restore on failure.
 */
export async function hideHomeRows(
  client: QueryClient,
  rows: readonly HomeRow[],
  now: number = Date.now(),
): Promise<HomeHide> {
  const added = rows.map((row) => ({
    ...row,
    settled: false,
    expiresAt: now + HIDE_LIMIT_MS,
  }));
  hidden = [...hidden, ...added];
  await client.cancelQueries(homeDays);
  const before = client.getQueriesData<InfiniteData<Worklist>>(homeDays);
  client.setQueriesData<InfiniteData<Worklist>>(homeDays, (data) =>
    isDay(data) ? marked(data) : data,
  );
  return {
    settle: () => {
      for (const row of added) row.settled = true;
    },
    restore: () => {
      hidden = hidden.filter((row) => !added.includes(row));
      for (const [key, data] of before) client.setQueryData(key, data);
    },
  };
}

/** Forgets every hidden row. For tests. */
export function forgetHiddenHomeRows(): void {
  hidden = [];
}
