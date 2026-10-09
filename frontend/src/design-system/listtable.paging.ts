// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which rendered page of a list table is on screen, and how many rows a page
// holds. A new narrowing sends the reader back to page one.

import { type RefObject, useEffect, useRef, useState } from "react";

/**
 * Page sizes the footer offers, as rendered pages. The caller fetches several
 * of them per read (`listFetchLimit`), and the table slices what it holds.
 *
 * The fetch is always a whole multiple of this, so the two stay in step. A
 * buffer sized on its own once made a list say "1-25 of 50 loaded so far".
 */
export const PAGE_SIZES = [25, 50, 100] as const;

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
      .sort((one, other) => (one === other ? 0 : one < other ? -1 : 1))
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
 * Not on arrival: a page read from the address must survive the first render.
 * So the reset compares the signature with the one seeded during the first
 * render. Counting effect runs fails, because StrictMode and late deps run
 * the effect on arrival more than once.
 */
function useResetOnNarrowing(narrowing: string, toFirstPage: () => void) {
  const narrowedBy = useRef(narrowing);
  // `toFirstPage` is re-made every render and must not re-run the effect;
  // what it does depends only on the signature.
  // biome-ignore lint/correctness/useExhaustiveDependencies: setPage is stable in effect
  useEffect(() => {
    if (narrowedBy.current === narrowing) {
      return;
    }
    narrowedBy.current = narrowing;
    toFirstPage();
    // The page and nothing else. This also fires on the way out, against an
    // address that is no longer the list's, and a scroll offset written then
    // would be lost. The pager scrolls to the top for its own moves instead.
  }, [narrowing]);
}

/** What the caller passes to own the page or the page size. */
export type PagingControl<Row> = Readonly<{
  rows: readonly Row[];
  page?: number;
  onPage?: (next: number) => void;
  perPage?: number;
  onPerPage?: (next: number) => void;
  hasMore: boolean;
  onLoadMore?: () => void;
}>;

/** The rows the table slices out, and the pager's state. */
export type Paging<Row> = Readonly<{
  current: number;
  lastPage: number;
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

/**
 * Slices the rows the caller holds into rendered pages. One read carries a
 * whole multiple of `perPage`, so the slices land on page boundaries the
 * reader reaches without a round trip each.
 */
export function usePaging<Row>(
  control: PagingControl<Row>,
  narrowing: Narrowing,
  scroller: RefObject<HTMLDivElement | null>,
): Paging<Row> {
  const { page, setPage, perPage, setPerPage } = usePageState(control);
  const lastPage = Math.max(1, Math.ceil(control.rows.length / perPage));
  const current = Math.min(page, lastPage);
  const from = (current - 1) * perPage;
  useResetOnNarrowing(narrowingSignature({ ...narrowing, perPage }), () =>
    setPage(1),
  );
  const goto = (to: number) => {
    // Stepping past what is loaded asks the server for the next cursor page.
    if (to > lastPage && control.hasMore) {
      control.onLoadMore?.();
    }
    setPage(Math.max(1, to));
    // The header is sticky and the body scrolls, so without this the reader
    // lands on page 2 already scrolled to its middle.
    if (scroller.current) {
      scroller.current.scrollTop = 0;
    }
  };
  return {
    current,
    lastPage,
    from,
    pageRows: control.rows.slice(from, from + perPage),
    perPage,
    setPerPage,
    goto,
  };
}
