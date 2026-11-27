// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which rendered page of a list table is on screen, and how many rows a page
// holds. A new narrowing sends the reader back to page one.

import { type RefObject, useEffect, useMemo, useRef, useState } from "react";
import { stable } from "../format/collate";

/**
 * Page sizes the footer offers, as rendered pages. The caller fetches a whole
 * number of them per read (`listFetchLimit`), by cursor or at a row offset.
 * The table slices what it holds on page boundaries.
 */
export const PAGE_SIZES = [25, 50, 100] as const;

export function fitsOnePage(
  rows: readonly unknown[],
  total: number | undefined,
  hasMore: boolean,
): boolean {
  return !hasMore && (total ?? rows.length) <= PAGE_SIZES[0];
}

/** Everything that narrows or reorders a list, read off its dials. */
export type Narrowing = Readonly<{
  search?: string;
  narrowKey?: string;
  chosen: Readonly<Record<string, string>>;
  sort?: string;
  archived?: boolean;
  scopeKey: string;
}>;

/**
 * Everything narrowing a list, as one value two renders can be compared by.
 *
 * Exported for the tests that pin what it calls a change.
 *
 * A caller that declares `narrowKey` has already named its narrowing. The rest
 * are read off the dials. `chosen`'s keys are sorted, because the same filters
 * in another insertion order are the same narrowing.
 */
export function narrowingSignature(
  dials: Narrowing & Readonly<{ perPage: number }>,
): string {
  const narrowedBy =
    dials.narrowKey ??
    Object.keys(dials.chosen)
      // Byte order, not the reader's: this identity is compared with itself a
      // render ago, so it must be the same string in every locale.
      .sort(stable)
      .map((name) => `${name}=${dials.chosen[name]}`)
      .join("&");
  return JSON.stringify([
    dials.search ?? "",
    narrowedBy,
    dials.perPage,
    dials.sort ?? "",
    dials.archived ?? false,
    dials.scopeKey,
  ]);
}

/**
 * Sends the reader back to page one when the narrowing changes. Page 2 of a
 * new filter, sort or scope holds rows they never asked for.
 *
 * Not when `keepsPage` says so: a link that changes the narrowing names its page.
 *
 * Not on arrival: a page read from the address must survive until the first
 * rows land. A scope that resolves meanwhile, as the deals pipeline does, is
 * part of arriving. Counting effect runs fails, because StrictMode and late
 * deps run the effect on arrival more than once.
 */
function useResetOnNarrowing(
  narrowing: string,
  toFirstPage: () => void,
  keepsPage: (() => boolean) | undefined,
  pending: boolean,
) {
  const narrowedBy = useRef(narrowing);
  const arrived = useRef(!pending);
  // `toFirstPage` is re-made every render and must not re-run the effect;
  // what it does depends only on the signature.
  // biome-ignore lint/correctness/useExhaustiveDependencies: setPage is stable in effect
  useEffect(() => {
    if (narrowedBy.current === narrowing) {
      return;
    }
    narrowedBy.current = narrowing;
    if (!arrived.current || keepsPage?.()) {
      return;
    }
    toFirstPage();
    // The page and nothing else. This also fires on the way out, against an
    // address that is no longer the list's, and a scroll offset written then
    // would be lost. The pager scrolls to the top for its own moves instead.
  }, [narrowing]);
  useEffect(() => {
    if (!pending) {
      arrived.current = true;
    }
  }, [pending]);
}

type Reach = Readonly<{
  enabled: boolean;
  /** The page asked for, and the last one the rows in hand fill. */
  page: number;
  lastPage: number;
  loaded: number;
  hasMore: boolean;
  /** Bumped by each press that steps past the rows, so a failed read retries. */
  asks: number;
  loadMore: () => void;
  setPage: (to: number) => void;
}>;

// Walks the cursor to a page past the rows in hand, for a list that cannot
// read a page at an offset. When the rows run out the address returns to the
// last page.
function useReachAskedPage(reach: Reach) {
  const past = reach.enabled && reach.page > reach.lastPage;
  // Re-armed by each arriving read or press; the callbacks are re-made every render.
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed on rows and presses
  useEffect(() => {
    if (!past) {
      return;
    }
    if (reach.hasMore) {
      reach.loadMore();
    } else if (reach.loaded > 0) {
      reach.setPage(reach.lastPage);
    }
  }, [past, reach.loaded, reach.hasMore, reach.asks]);
}

/**
 * Rows the caller read at a row offset for a page past the rows in hand. It
 * is there while the page is past them, so the cursor does not walk.
 */
export type JumpedRows<Row> = Readonly<{
  /** The offset of the first row. */
  from: number;
  rows: readonly Row[];
  /** False while the read, or the walk on from it, is under way. */
  settled: boolean;
}>;

/** What the caller passes to own the page or the page size. */
export type PagingControl<Row> = Readonly<{
  rows: readonly Row[];
  page?: number;
  onPage?: (next: number) => void;
  perPage?: number;
  onPerPage?: (next: number) => void;
  hasMore: boolean;
  onLoadMore?: () => void;
  /** How many rows match on the server, when it counts them. */
  total?: number;
  jumped?: JumpedRows<Row>;
  /** False where a body outside the table owns the paging; default true. */
  reachAsked?: boolean;
  /** True when the page in hand was named with the narrowing, so no reset. */
  keepsPage?: () => boolean;
  /** The list's rows have not arrived yet. */
  pending?: boolean;
}>;

/** The rows the table slices out, and the pager's state. */
export type Paging<Row> = Readonly<{
  current: number;
  /** The last page the pager numbers, which a count puts past the rows in hand. */
  lastPage: number;
  /** `lastPage` is the list's real end: counted, or every row is in hand. */
  endKnown: boolean;
  /** The page a press or an address named, while its rows are read. */
  asked?: number;
  /** The rows have not arrived, or the page on screen is a jump still being read. */
  pending: boolean;
  from: number;
  pageRows: readonly Row[];
  perPage: number;
  setPerPage: (next: number) => void;
  goto: (to: number) => void;
}>;

/**
 * The page is the table's own unless the caller passes one, which a screen
 * does when the page lives in the address. The page size splits the same way,
 * on the handler: a caller that cannot be told of a change cannot own it.
 */
function usePageState<Row>(control: PagingControl<Row>) {
  const [ownPage, setOwnPage] = useState(1);
  const setPage = (to: number) => {
    setOwnPage(to);
    control.onPage?.(to);
  };
  // Without a handler there is no wire to re-ask, so the dial slices the rows
  // already in hand. A disabled dial read as a broken one.
  const [ownPerPage, setOwnPerPage] = useState(
    control.perPage ?? PAGE_SIZES[0],
  );
  const perPage = control.onPerPage
    ? (control.perPage ?? PAGE_SIZES[0])
    : ownPerPage;
  return {
    page: control.page ?? ownPage,
    setPage,
    perPage,
    setPerPage: control.onPerPage ?? setOwnPerPage,
  };
}

/** The pages the list holds, and the one on screen within them. */
function pageBounds<Row>(
  control: PagingControl<Row>,
  page: number,
  perPage: number,
) {
  const { jumped } = control;
  const inHand = Math.max(
    control.rows.length,
    jumped?.rows.length ? jumped.from + jumped.rows.length : 0,
  );
  const loadedPages = Math.max(1, Math.ceil(inHand / perPage));
  // A count is read once, so rows added since can outrun it; the rows in hand win.
  const counted =
    control.hasMore && control.total !== undefined
      ? Math.max(loadedPages, Math.ceil(control.total / perPage))
      : undefined;
  // A jumped read is under way before the count arrives, so the page waits for it.
  const reachable = jumped ? (counted ?? page) : loadedPages;
  return { loadedPages, counted, current: Math.min(page, reachable) };
}

/** The rows of the page starting at `from`, from whichever read holds them. */
function sliceOf<Row>(
  rows: readonly Row[],
  jumped: JumpedRows<Row> | undefined,
  from: number,
  perPage: number,
): readonly Row[] {
  if (jumped && from >= jumped.from) {
    return jumped.rows.slice(from - jumped.from, from - jumped.from + perPage);
  }
  return rows.slice(from, from + perPage);
}

// A jumped page the count no longer reaches, as after rows were removed, goes
// to the last page the count names.
function useClampJumpedPage(
  jumped: boolean,
  page: number,
  counted: number | undefined,
  setPage: (to: number) => void,
) {
  // biome-ignore lint/correctness/useExhaustiveDependencies: setPage is re-made every render
  useEffect(() => {
    if (jumped && counted !== undefined && page > counted) {
      setPage(counted);
    }
  }, [jumped, page, counted]);
}

/**
 * Slices the rows the caller holds into rendered pages. One read carries a
 * whole multiple of `perPage`, so the slices land on page boundaries.
 */
export function usePaging<Row>(
  control: PagingControl<Row>,
  narrowing: Narrowing,
  scrollerRef: RefObject<HTMLDivElement | null>,
): Paging<Row> {
  const { page, setPage, perPage, setPerPage } = usePageState(control);
  const { loadedPages, counted, current } = pageBounds(control, page, perPage);
  const from = (current - 1) * perPage;
  const { rows, jumped } = control;
  const pageRows = useMemo(
    () => sliceOf(rows, jumped, from, perPage),
    [rows, jumped, from, perPage],
  );
  const [asks, setAsks] = useState(0);
  useReachAskedPage({
    enabled: control.reachAsked !== false && jumped === undefined,
    page,
    lastPage: loadedPages,
    loaded: control.rows.length,
    hasMore: control.hasMore,
    asks,
    loadMore: () => control.onLoadMore?.(),
    setPage,
  });
  useClampJumpedPage(jumped !== undefined, page, counted, setPage);
  useResetOnNarrowing(
    narrowingSignature({ ...narrowing, perPage }),
    () => setPage(1),
    control.keepsPage,
    control.pending ?? false,
  );
  const goto = (to: number) => {
    // Stepping past what is loaded is answered by `useReachAskedPage`.
    if (to > loadedPages) {
      setAsks((presses) => presses + 1);
    }
    setPage(Math.max(1, to));
    // The header is sticky and the body scrolls, so without this the reader
    // lands on page 2 already scrolled to its middle.
    if (scrollerRef.current) {
      scrollerRef.current.scrollTop = 0;
    }
  };
  const reading = jumped
    ? !jumped.settled
    : control.hasMore && page > loadedPages;
  return {
    current,
    lastPage: counted ?? loadedPages,
    endKnown: counted !== undefined || !control.hasMore,
    asked: reading ? page : undefined,
    pending: (control.pending ?? false) || (jumped !== undefined && reading),
    from,
    pageRows,
    perPage,
    setPerPage,
    goto,
  };
}

/**
 * The slots of a strip longer than it: page one, a gap, the pages around the
 * current one, a gap, the last. A desktop has room for seven, a narrower screen five.
 */
export const PAGER_SLOTS = { wide: 7, short: 5 } as const;

/** A slot in the pager: a page to jump to, or a gap where pages were skipped. */
export type PagerSlot = number | "gap";

const pages = (first: number, last: number): PagerSlot[] =>
  Array.from({ length: last - first + 1 }, (_, index) => first + index);

/**
 * Page one stays reachable because a lost reader goes back there. The last
 * page is numbered so the strip says how long the list is. A longer list
 * fills the same `slots` at every page, so Next never slides out between
 * clicks. An `open` list has pages past `lastPage`, so a gap follows them.
 */
export function pagerSlots(
  current: number,
  lastPage: number,
  open = false,
  slots: number = PAGER_SLOTS.wide,
): PagerSlot[] {
  const tail: PagerSlot[] = open ? ["gap"] : [];
  if (lastPage <= slots) {
    return [...pages(1, lastPage), ...tail];
  }
  // At either end the window widens to the gap, so the strip keeps its count.
  const end = slots - 2;
  const reach = Math.floor((slots - 4) / 2);
  if (current + reach <= end) {
    return [...pages(1, end), "gap", lastPage, ...tail];
  }
  if (current - reach > lastPage - end) {
    return [1, "gap", ...pages(lastPage - end + 1, lastPage), ...tail];
  }
  return [
    1,
    "gap",
    ...pages(current - reach, current + reach),
    "gap",
    lastPage,
    ...tail,
  ];
}
