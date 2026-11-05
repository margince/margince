// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  type QueryKey,
  type UseQueryResult,
  useQueries,
  useQueryClient,
} from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { CLOSE_DAY_DIAL } from "../../lib/dealranges";
import {
  type DealFilters,
  type DealListPage,
  type DealListQuery,
  dealsQueryParams,
  nextCursorOf,
  readDealPage,
  useCloseDay,
} from "../../lib/deals.queries";

type Deal = components["schemas"]["Deal"];

// A board column's page: smaller than the table's, since every column reads at once.
export const STAGE_COLUMN_LIMIT = 25;

/** One stage column of the board, in the shape QueryStates and LoadMoreButton read. */
export type StageColumnRead = Readonly<{
  deals: Deal[];
  isPending: boolean;
  /** The first page failed, so the column holds no cards. */
  isError: boolean;
  error: unknown;
  refetch: () => unknown;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => unknown;
  /** A later read failed; the cards already read stay. */
  loadMoreError: unknown;
}>;

// The pages a column's "Load more" asked for, by cache entry. The click leaves
// its wish here, so the walk reads it at fetch time and closes over no render.
const askedPages = new WeakMap<object, number>();

// Each cursor is the one the page before it answered, so a walk's pages meet
// with no gap or overlap.
async function walkStagePages(
  query: DealListQuery,
  depth: number,
  seed: readonly DealListPage[],
  signal: AbortSignal,
): Promise<DealListPage[]> {
  const pages = [...seed];
  const seedEnd = seed.at(-1);
  let cursor = seedEnd ? nextCursorOf(seedEnd) : undefined;
  while (pages.length < depth && (pages.length === 0 || cursor !== undefined)) {
    const page = await readDealPage({ ...query, cursor }, signal);
    pages.push(page);
    cursor = nextCursorOf(page);
  }
  return pages;
}

// A column re-reads as many pages as its entry holds, plus one when asked. Load
// more builds on held pages only while fresh: a write or failed read stales them.
async function readStageColumn(
  client: QueryClient,
  queryKey: QueryKey,
  query: DealListQuery,
  signal: AbortSignal,
): Promise<DealListPage[]> {
  const entry = client
    .getQueryCache()
    .find<DealListPage[]>({ queryKey, exact: true });
  const held = entry?.state.data ?? [];
  const asked = (entry && askedPages.get(entry)) ?? 0;
  const grows = asked > held.length && entry?.state.isInvalidated === false;
  const pages = await walkStagePages(
    query,
    Math.max(1, held.length, asked),
    grows ? held : [],
    signal,
  );
  if (entry && askedPages.get(entry) === asked) askedPages.delete(entry);
  return pages;
}

function askForMore(
  client: QueryClient,
  queryKey: QueryKey,
  refetch: () => unknown,
) {
  const entry = client
    .getQueryCache()
    .find<DealListPage[]>({ queryKey, exact: true });
  if (!entry) return;
  askedPages.set(entry, (entry.state.data?.length ?? 0) + 1);
  return refetch();
}

// The newest copy of each deal, by version. Mid-refetch a moved deal can sit in
// the column it left and the one it joined; it draws once, where it now is.
function newestOfEach(deals: readonly Deal[]): Deal[] {
  const byId = new Map<string, Deal>();
  for (const deal of deals) {
    const held = byId.get(deal.id);
    if (!held || (deal.version ?? 0) > (held.version ?? 0))
      byId.set(deal.id, deal);
  }
  return [...byId.values()];
}

function stageColumnRead(
  query: UseQueryResult<DealListPage[]>,
  loadMore: () => unknown,
): StageColumnRead {
  const shown = query.data;
  const last = shown?.at(-1);
  const failed = query.isError ? query.error : null;
  return {
    deals: shown?.flatMap((page) => page.data) ?? [],
    isPending: shown === undefined && query.isPending,
    isError: shown === undefined && query.isError,
    error: shown === undefined ? failed : null,
    refetch: () => query.refetch(),
    hasNextPage: last !== undefined && nextCursorOf(last) !== undefined,
    // Paused offline counts as fetching, so a second press cannot stack.
    isFetchingNextPage: query.fetchStatus !== "idle",
    fetchNextPage: loadMore,
    loadMoreError: shown === undefined ? null : failed,
  };
}

// Not one walk over every stage: a later column would head a count over no cards.
// Not useInfiniteQuery: useQueries takes no infinite query.
// A column shows, and re-reads, the pages its cache entry holds.
export function useStageColumns(
  f: DealFilters,
  stageIds: readonly string[],
  enabled = true,
) {
  const closeDay = useCloseDay(f.filters[CLOSE_DAY_DIAL]);
  const client = useQueryClient();
  const readStages = f.filters.stage_id
    ? stageIds.filter((stageId) => stageId === f.filters.stage_id)
    : stageIds;
  const keyOf = (stageId: string) =>
    ["deals", "stage-column", f, closeDay, stageId] as const;
  const queries = useQueries({
    queries: readStages.map((stageId) => ({
      queryKey: keyOf(stageId),
      enabled,
      queryFn: ({
        client,
        queryKey,
        signal,
      }: {
        client: QueryClient;
        queryKey: QueryKey;
        signal: AbortSignal;
      }) =>
        readStageColumn(
          client,
          queryKey,
          {
            ...dealsQueryParams({ ...f, closeDay }),
            limit: STAGE_COLUMN_LIMIT,
            stage_id: stageId,
          },
          signal,
        ),
    })),
  });
  const columns = new Map<string, StageColumnRead>();
  readStages.forEach((stageId, index) => {
    const query = queries[index];
    if (!query) return;
    columns.set(
      stageId,
      stageColumnRead(query, () =>
        askForMore(client, keyOf(stageId), () => query.refetch()),
      ),
    );
  });
  const reads = [...columns.values()];
  return {
    columns,
    deals: newestOfEach(reads.flatMap((column) => column.deals)),
    isPending: reads.some((column) => column.isPending),
    error: queries.find((query) => query.isError)?.error ?? null,
  };
}
