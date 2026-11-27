// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { FIRST_PAGE } from "../api/client";
import type { JumpedRows } from "../design-system/listtable.paging";
import type { ListPage } from "./listquery";
import {
  LIST_OFFSET_CAP,
  type ListReach,
  listFetchLimit,
} from "./listqueryparams";

/** The read that holds a page past the cursor's rows, and the page within it. */
export type JumpTarget = Readonly<{
  /** Where the read starts: a read boundary at or before the page, or the cap. */
  at: number;
  pastCap: boolean;
  /** The offset of the page's first row, and its size. */
  from: number;
  perPage: number;
}>;

/** How far the cursor's own reads have come. */
export type CursorPrefix = Readonly<{
  rows: number;
  hasMore: boolean;
  pending: boolean;
}>;

/**
 * The read `page` needs when the cursor's rows do not hold it, or undefined
 * when they do or will. A jump starts on a read boundary, so the page after
 * it lands in the same read and Next costs no wait.
 */
export function jumpTarget(
  page: number,
  perPage: number,
  prefix: CursorPrefix,
): JumpTarget | undefined {
  const from = (page - 1) * perPage;
  const span = listFetchLimit(perPage);
  const held = from < span || from + perPage <= prefix.rows;
  if (held || (!prefix.pending && !prefix.hasMore)) {
    return undefined;
  }
  const at = from - (from % span);
  return at <= LIST_OFFSET_CAP
    ? { at, pastCap: false, from, perPage }
    : { at: LIST_OFFSET_CAP, pastCap: true, from, perPage };
}

export type ListJumpRead<Row> = (
  cursor: string | null,
  reach: ListReach,
  signal: AbortSignal,
) => Promise<ListPage<Row>>;

/**
 * Reads the page `target` names at its offset, beside the cursor's own pages,
 * which stay one unbroken run from the first row. Past `LIST_OFFSET_CAP` it
 * reads at the cap and walks the cursor on from there to the page.
 *
 * `prefixKey` is the cursor read's query key, so whatever refreshes that list
 * refreshes its jumps too.
 */
export function useListJump<Row>({
  prefixKey,
  target,
  read,
  enabled,
}: Readonly<{
  prefixKey: readonly unknown[];
  target: JumpTarget | undefined;
  read: ListJumpRead<Row>;
  enabled: boolean;
}>) {
  const live = enabled ? target : undefined;
  const at = live?.at ?? 0;
  const pastCap = live?.pastCap ?? false;
  const jump = useInfiniteQuery({
    queryKey: [...prefixKey, { at, pastCap }],
    queryFn: ({ pageParam, signal }) =>
      read(
        pageParam,
        pageParam === FIRST_PAGE ? { offset: at, pastCap } : { pastCap },
        signal,
      ),
    initialPageParam: FIRST_PAGE,
    getNextPageParam: (last: ListPage<Row>) =>
      last.page.has_more && last.page.next_cursor
        ? last.page.next_cursor
        : undefined,
    enabled: live !== undefined,
  });
  const pages = jump.data?.pages ?? [];
  const rows = pages.flatMap((page) => page.data);
  // A failed read stops the walk; the list shows the failure, and its retry re-reads.
  const walking =
    live?.pastCap &&
    !jump.isError &&
    rows.length < live.from + live.perPage - at &&
    jump.hasNextPage;
  const { fetchNextPage, isFetching } = jump;
  // Re-armed by each arriving read, which can land in the render it started in.
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed on rows read
  useEffect(() => {
    if (walking && !isFetching) {
      fetchNextPage();
    }
  }, [walking, isFetching, fetchNextPage, rows.length]);
  const jumped: JumpedRows<Row> | undefined = live && {
    from: at,
    rows,
    settled: !jump.isPending && !walking,
  };
  return {
    jumped,
    /** The jump counts like a first page, so it knows the total on its own. */
    total: pages[0]?.page.total,
    isError: live !== undefined && jump.isError,
    error: live ? jump.error : null,
    refetch: () => {
      if (live) {
        jump.refetch();
      }
    },
  };
}

/** The cursor's read as the list holds it, for a jump to stand beside. */
export type CursorRead<Row> = Readonly<{
  rows: readonly Row[];
  hasMore: boolean;
  pending: boolean;
  total?: number;
  isError: boolean;
  error: unknown;
  refetch: () => void;
}>;

/**
 * The cursor's rows and the jump's, each record once by `keyOf`: what a
 * selection over the page on screen can hold.
 */
export function heldRows<Row>(
  rows: readonly Row[],
  jumped: readonly Row[] | undefined,
  keyOf: ((row: Row) => string) | undefined,
): readonly Row[] {
  if (!jumped?.length || !keyOf) {
    return rows;
  }
  const keys = new Set(rows.map(keyOf));
  return [...rows, ...jumped.filter((row) => !keys.has(keyOf(row)))];
}

/**
 * Reads the page the address names at its offset when `cursor` does not hold
 * it. Returns every row once, the count, and either read's failure and retry.
 */
export function useJumpedList<Row>({
  prefixKey,
  read,
  enabled,
  page,
  perPage,
  rowKey,
  cursor,
}: Readonly<{
  prefixKey: readonly unknown[];
  read: ListJumpRead<Row>;
  enabled: boolean;
  page: number;
  perPage: number;
  rowKey: ((row: Row) => string) | undefined;
  cursor: CursorRead<Row>;
}>) {
  const jump = useListJump({
    prefixKey,
    target: jumpTarget(page, perPage, {
      rows: cursor.rows.length,
      hasMore: cursor.hasMore,
      pending: cursor.pending,
    }),
    read,
    enabled,
  });
  return {
    jumped: jump.jumped,
    held: heldRows(cursor.rows, jump.jumped?.rows, rowKey),
    total: jump.total ?? cursor.total,
    isError: cursor.isError || jump.isError,
    error: cursor.error ?? jump.error,
    refetch: () => {
      cursor.refetch();
      jump.refetch();
    },
  };
}
