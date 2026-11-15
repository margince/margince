// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { infiniteQueryOptions } from "@tanstack/react-query";
import { FIRST_PAGE } from "../api/client";
import type { ListPage } from "./listquery";
import { LIST_PAGE_SIZES, type ListQuery } from "./listqueryparams";

export type ListFetchPage<Row> = (
  query: ListQuery,
  cursor: string | null,
  signal: AbortSignal,
) => Promise<ListPage<Row>>;

/** The state a list opens on before a reader has turned any dial. */
export function listOpening(
  initialSort: string | undefined,
  initialFilters: Readonly<Record<string, string>> | undefined,
): ListQuery {
  return {
    q: "",
    sort: initialSort ?? "",
    includeArchived: false,
    filters: initialFilters ?? {},
    perPage: LIST_PAGE_SIZES[0],
  };
}

/**
 * The cache entry and read for one list query. useListQuery mounts it, and a
 * navigation prefetch warms it, so both find the same key and the same pages.
 *
 * `readerScope` is `healthScope(me)`: who reads, in which workspace, under
 * which grants and row scope. The rows a list holds depend on all of it. A
 * same-user grant change keeps record caches. Without the scope in the key, a
 * list would go on serving the rows the old grants allowed.
 */
export function listInfiniteOptions<Row>(
  key: string,
  readerScope: string,
  query: ListQuery,
  fetchPage: ListFetchPage<Row>,
) {
  return infiniteQueryOptions({
    queryKey: [key, readerScope, query],
    queryFn: ({ pageParam, signal }) => fetchPage(query, pageParam, signal),
    initialPageParam: FIRST_PAGE,
    getNextPageParam: (last: ListPage<Row>) =>
      last.page.has_more && last.page.next_cursor
        ? last.page.next_cursor
        : undefined,
  });
}
