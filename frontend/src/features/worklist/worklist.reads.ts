import {
  infiniteQueryOptions,
  useInfiniteQuery,
  useQuery,
} from "@tanstack/react-query";
import { api } from "../../api/client";
import { unwrap } from "../../lib/problem";
import { isLiveWorkRead, worklistKey } from "../../lib/workkeys";
import type {
  TeamBoard,
  TeamExceptions,
  Worklist,
  WorklistFilter,
  WorklistItem,
  WorklistScope,
} from "./worklist.queries";
import { represented } from "./worklistunits";

export { useHandledForYou } from "../../lib/handled";
export { worklistKey } from "../../lib/workkeys";

// The whole day in one read, at one scope or one named owner, and one filter.
//
// The key carries every dial, so switching any of them is a different query
// rather than a refetch. The reader can move back to a view already loaded
// without watching it reassemble.
//
// A named owner narrows the day to one rep's queue and outranks the scope word.
// "Their queue" is narrower than any of mine/team/all, and the server answers
// 422 for the pair rather than guessing. So the scope travels only when nobody
// is named.
export function useWorklist(
  scope: WorklistScope,
  filter: WorklistFilter,
  owner?: string,
  enabled = true,
) {
  return useInfiniteQuery({
    ...worklistDayOptions(scope, filter, owner),
    enabled,
  });
}

/** The read useWorklist makes, which Home's prefetch also starts. */
export function worklistDayOptions(
  scope: WorklistScope,
  filter: WorklistFilter,
  owner?: string,
) {
  const queryKey = [...worklistKey, scope, filter, owner ?? ""];
  return infiniteQueryOptions({
    queryKey,
    ...(!isLiveWorkRead(queryKey) ? { refetchOnWindowFocus: true } : {}),
    initialPageParam: undefined as string | undefined,
    // React Query aborts the signal once no screen still observes this key, and
    // the server then stops assembling the day instead of answering nobody.
    queryFn: ({ pageParam, signal }) =>
      readWorklistPage(scope, filter, owner, pageParam, signal),
    // A page can empty as rows leave the snapshot; only its cursor ends the walk.
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

/** One page of the day at those dials, the request `useWorklist` makes. */
export async function readWorklistPage(
  scope: WorklistScope,
  filter: WorklistFilter,
  owner: string | undefined,
  cursor: string | undefined,
  signal?: AbortSignal,
): Promise<Worklist> {
  const dials = owner ? { owner, filter } : { scope, filter };
  return unwrap(
    await api.GET("/worklist", {
      params: { query: cursor ? { ...dials, cursor } : dials },
      signal,
    }),
  );
}

/**
 * The rows loaded so far, as one queue.
 *
 * Deduped, and that is a correctness requirement rather than defensive tidying.
 * The contract says a walk offers no snapshot: the day is re-assembled and
 * re-ranked on every read. A row crossing the page boundary between two reads
 * is served twice or not at all. Twice is the case a client can fix, and React
 * would otherwise render duplicate keys for it.
 *
 * By source and id, the same identity the screen selects by: `id` alone is not
 * unique across lanes.
 */
export function loadedQueue(pages: readonly Worklist[]): WorklistItem[] {
  const seen = new Set<string>();
  const rows: WorklistItem[] = [];
  for (const page of pages) {
    for (const item of page.queue) {
      const identity = `${item.source}-${item.id}`;
      if (seen.has(identity)) {
        continue;
      }
      seen.add(identity);
      rows.push(item);
    }
  }
  return rows;
}

/**
 * What is going wrong on this lead's team.
 *
 * `enabled` for the reason the board takes one. The read is gated to the lead
 * tier, so asking it as a rep earns a 403 the screen would have to render as
 * something. A caller that cannot hold the tier does not ask.
 */
export function useTeamExceptions(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: [...worklistKey, "exceptions"],
    queryFn: async (): Promise<TeamExceptions> => {
      return unwrap(await api.GET("/worklist/exceptions", {}));
    },
  });
}

// Who on the team is carrying what.
//
// Its own read rather than a widening of the queue above, because that queue
// assembles one rep's day. The per-user sources are never read for anybody
// else, so no scope could produce a colleague's rows. This answers counts.
// Pressing a row opens that rep's day through the owner above, the drill-down
// the board exists to route to.
//
// `enabled` carries the reader's tier, taken from the queue's own scope_options
// so the two cannot disagree. A seat that may not ask for `team` is refused this
// endpoint as well. Fetching anyway would put a 403 behind a surface the reader
// was never shown a way into.
export function useTeamBoard(enabled: boolean, team?: string) {
  return useQuery({
    enabled,
    queryKey: [...worklistKey, "team", team ?? ""],
    queryFn: async (): Promise<TeamBoard> => {
      return unwrap(
        await api.GET("/worklist/team", {
          params: { query: { team } },
        }),
      );
    },
  });
}

/** Preserve the server's order across the pages the reader has requested. */
export function accumulatedDay(
  pages: readonly Worklist[] | undefined,
): Worklist | undefined {
  const first = pages?.[0];
  if (!first || !pages) return undefined;
  const queue = loadedQueue(pages);
  const counts = first.counts.map((entry) => ({
    ...entry,
    more_available: pages.some((page) =>
      page.counts.some(
        (count) => count.category === entry.category && count.more_available,
      ),
    ),
    shown: represented(queue.filter((item) => item.category === entry.category))
      .items,
  }));
  const unavailable = new Map(
    pages
      .flatMap((page) => page.sources_unavailable)
      .map((entry) => [entry.source, entry]),
  );
  return {
    ...first,
    queue,
    counts,
    reach: first.reach?.map((entry) => ({
      ...entry,
      more_available: pages.some((page) =>
        page.reach?.some(
          (source) => source.source === entry.source && source.more_available,
        ),
      ),
    })),
    sources_unavailable: [...unavailable.values()],
    readings: {
      ...first.readings,
      more_available: pages.some((page) => page.readings.more_available),
    },
    next_cursor: pages.at(-1)?.next_cursor,
  };
}
