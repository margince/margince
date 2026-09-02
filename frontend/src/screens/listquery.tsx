import { useInfiniteQuery } from "@tanstack/react-query";
import {
  type Dispatch,
  type ReactNode,
  type SetStateAction,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { FIRST_PAGE } from "../api/client";
import { navigate, type Route, routeHash, useHash } from "../app/router";
import { useScrollMemory } from "../app/scrollmemory";
import { currentParams, type UrlParams, useUrlParams } from "../app/urlstate";
import { Button } from "../design-system/atoms";
import {
  type ListChip,
  type ListColumn,
  type ListSelection,
  ListTable as ListSurface,
  type ListView,
} from "../design-system/listtable";
import { stable } from "../format/collate";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, useMe, useSorMode } from "./common";
import { rosterReading, useRoster, useRosterPartial } from "./entityref";
import { useTagVocabulary } from "./tags.queries";

// The shared list foundation (P-14): every list screen sends the rich
// q/sort/cursor/include_archived/filter vocabulary instead of a flat
// limit:50, and paginates by keyset (never offset — the workspace's rows
// mutate under a live feed). useListQuery owns the react-query wiring;
// ListTable binds that query to the list surface, which owns the controls.

/**
 * How long a typed filter settles before it becomes a request.
 *
 * Exported because it is a product decision about what a keystroke costs, not
 * a detail of this file: any surface that turns typing into a server query
 * reads it here rather than picking its own number, so the app has one answer
 * to "how responsive is a filter" instead of one per screen.
 */
export const SEARCH_DEBOUNCE_MS = 250;

export type ListQuery = {
  q: string;
  sort: string;
  includeArchived: boolean;
  filters: Record<string, string>;
  /**
   * Rows per page, chosen by the reader in the table footer. It is the size of
   * a RENDERED page, not of a read: one read fetches several of these at once
   * (see `listFetchLimit`) so the pager can offer them as numbers the reader
   * reaches without waiting.
   */
  perPage: number;
};

/** The page sizes the footer offers; the first is the default. */
export const LIST_PAGE_SIZES = [25, 50, 100] as const;

/**
 * The page size `value` names, or undefined for anything else.
 *
 * An address is text a person can edit, so `per=7` and `per=banana` are both
 * ordinary. Neither is an error worth showing anybody — the list simply opens
 * at the size it would have opened at — but neither may become a request
 * either, because `listFetchLimit` divides by it.
 */
function pageSizeOf(value: string | undefined): number | undefined {
  return LIST_PAGE_SIZES.find((size) => String(size) === value);
}

/**
 * The address prefix a list uses when it is not the only one on its route.
 *
 * Two lists on one route share one address, and the flat vocabulary below then
 * describes both at once: the settings Data-model tab draws the product table
 * and the offer-template table side by side, so sorting products by `sku` sent
 * `sort=sku` to `/offer-templates` (a 422 the reader never asked for) and the
 * product `active` chip narrowed the templates read as well. A scope answers
 * that by naming every dial the list owns — `products.q`, `products.sort` — so
 * the two lists on one route hold two parameter spaces.
 *
 * Undefined, and the dials keep their bare names. That is not laziness: a bare
 * `#/companies?q=acme` is a link people paste, and prefixing every list in the
 * product would break every one of them for no reader's benefit. A scope is for
 * the lists that genuinely share an address, and only those.
 *
 * The two spaces are told apart by the DOT, which is why a wire filter may not
 * contain one: a scoped list reads only the keys carrying its own prefix, and an
 * unscoped list reads only the keys carrying no prefix at all. Neither writes
 * over the other's.
 */
function scoped(scope: string | undefined, name: string): string {
  return scope ? `${scope}.${name}` : name;
}

/** Is `key` a dial of the list at `scope`, rather than another list's? */
function ownedBy(key: string, scope: string | undefined): boolean {
  return scope ? key.startsWith(`${scope}.`) : !key.includes(".");
}

/**
 * `live` with every dial the list at `scope` owns replaced by `mine`.
 *
 * A list writes its whole state at once, so without this the second list on a
 * route would erase the first one's dials with every keystroke. For a list that
 * owns the address alone this is the plain replacement it always was, since
 * every key is then its own.
 */
function replaceOwn(
  live: UrlParams,
  mine: UrlParams,
  scope: string | undefined,
): UrlParams {
  const next = new Map<string, string>();
  for (const [key, value] of live) {
    if (!ownedBy(key, scope)) {
      next.set(key, value);
    }
  }
  for (const [key, value] of mine) {
    next.set(key, value);
  }
  return next;
}

/**
 * The list's own dials, and the names they go by in the address.
 *
 * `q`, `sort` and `include_archived` are the WIRE's own spellings, because a
 * filter chip's key already IS a wire param name — `...query.filters` is spread
 * straight onto the request — so the address and the endpoint describe a
 * narrowed list in one vocabulary instead of two that have to be kept in step.
 * Every other key in the address is therefore a filter, with no list of them
 * kept here: a screen that adds a chip gets an addressable one for free, and a
 * table of known filters would be a second copy of every screen's chip set.
 *
 * `per` is the exception and has no wire spelling, because a rendered page size
 * is a choice about drawing rather than about which rows exist. It is the
 * list's own name, which is why a filter param may not be called `per`.
 */
// Which RENDERED page is on screen. The list's own name, not a wire one: the
// server is paged by keyset cursor, and a page NUMBER is a slice of what has
// already been fetched (see listFetchLimit) rather than something to ask it for.
// A stale cursor in an address would 422; a stale page number simply shows the
// last page there is.
const PAGE_PARAM = "page";

/**
 * The rendered page `params` names, or 1 for an address naming none or junk.
 *
 * Exported for the ONE screen that composes this codec by hand rather than
 * through `useListQuery` — the deals screen drives a board and a table from a
 * single query, and reaches for these pieces instead of growing a second answer
 * to what a narrowed list's address looks like.
 */
export function listPageOf(params: UrlParams, scope?: string): number {
  const asked = Number(params.get(scoped(scope, PAGE_PARAM)));
  return Number.isInteger(asked) && asked > 1 ? asked : 1;
}

/** `params` with the rendered page set, page one being spelled by absence. */
export function withListPage(
  params: UrlParams,
  page: number,
  scope?: string,
): UrlParams {
  const next = new Map(params);
  const name = scoped(scope, PAGE_PARAM);
  if (page > 1) {
    next.set(name, String(page));
  } else {
    next.delete(name);
  }
  return next;
}

const PER_PAGE_PARAM = "per";

/** A screen with no drawing dial of its own, which is most of them. */
const NO_SCREEN_DIALS: readonly string[] = [];

const LIST_OWN_PARAMS: ReadonlySet<string> = new Set([
  "q",
  "sort",
  "include_archived",
  PER_PAGE_PARAM,
  PAGE_PARAM,
]);

/**
 * This list's dials in `params`, keyed by their BARE names.
 *
 * One reading for both spellings: a scoped list's `products.q` and an unscoped
 * list's `q` arrive here as `q`, so everything below is written once against the
 * vocabulary rather than twice against two prefixes. Another list's keys are not
 * in the result at all, which is what keeps the second table on a route from
 * reading the first one's filters as its own.
 */
function ownDials(
  params: UrlParams,
  scope: string | undefined,
): Map<string, string> {
  const mine = new Map<string, string>();
  const prefix = scope ? `${scope}.` : "";
  for (const [key, value] of params) {
    if (ownedBy(key, scope)) {
      mine.set(key.slice(prefix.length), value);
    }
  }
  return mine;
}

/**
 * The address keys a SCREEN owns, held out of the list's parameter space.
 *
 * The codec treats every key it does not recognise as a wire filter, which is
 * the right default — it is how a chip a screen adds becomes addressable with no
 * code here. A screen's own DRAWING dial is the exception: board or table, which
 * pipeline's board, decides how the same rows are presented and nothing about
 * which rows exist. Left in the parameter space it was spread onto the request
 * (`GET /leads?view=board` really went out), counted as a narrowing, and wiped
 * by "clear filters" — pressing which flipped the board back to a table.
 *
 * ONE pair, because this was two: the deals board grew its own copies of these
 * two functions, the leads queue did not grow them at all, and that is exactly
 * the difference between the two screens' defects. A screen names its drawing
 * dials once and the codec holds them apart.
 */
export function withoutScreenDials(
  params: UrlParams,
  dials: readonly string[],
): UrlParams {
  if (dials.length === 0) {
    return params;
  }
  const rest = new Map(params);
  for (const dial of dials) {
    rest.delete(dial);
  }
  return rest;
}

/**
 * `listDials` with the screen's own dials carried over from `carrying`.
 *
 * A dial the list dials ALREADY name wins over the live address. A saved deal
 * view records the pipeline it was saved on, so applying one writes that
 * pipeline through the list's own filter path; carrying the live address over
 * the top of it left the reader looking at another board's stages with the
 * saved view highlighted.
 */
export function mergeScreenDials(
  listDials: UrlParams,
  carrying: UrlParams,
  dials: readonly string[],
): UrlParams {
  if (dials.length === 0) {
    return listDials;
  }
  const merged = new Map(listDials);
  for (const dial of dials) {
    const value = carrying.get(dial);
    if (value && !merged.has(dial)) {
      merged.set(dial, value);
    }
  }
  return merged;
}

/**
 * The query `params` describes, with `opening` standing where it says nothing.
 *
 * An address that carries any dial at all is the WHOLE truth about the list:
 * filters are replaced rather than merged, because a reader who cleared the
 * owner chip and shared the link must not have it put back by the screen's
 * default. `opening` therefore only stands in for an address that says nothing
 * whatsoever, and only until this list has written its own opening state there.
 */
export function listQueryFromParams(
  params: UrlParams,
  opening: ListQuery,
  /**
   * Whether this list has already written its opening state to the address.
   *
   * Until it has, a bare address means "just arrived" and the screen's own
   * answers apply. Once it has, a bare address means the reader cleared
   * everything — which is a different list, and on a screen that opens
   * pre-narrowed (the leads queue opens on a rep's OWN leads) it is the whole
   * behaviour of the owner chip's "all".
   */
  seeded: boolean,
  /** The address prefix this list's dials carry; see `scoped`. */
  scope?: string,
): ListQuery {
  const mine = ownDials(params, scope);
  const filters: Record<string, string> = {};
  for (const [key, value] of mine) {
    if (!LIST_OWN_PARAMS.has(key)) {
      filters[key] = value;
    }
  }
  // An address carrying a dial, or a reader who has turned one, has EXPRESSED
  // what the list should be. Only a reader who has expressed nothing at all
  // gets the screen's opening answers.
  //
  // THIS list's dials, not the address's size: on a route holding two lists the
  // other one seeds first, and counting its keys would tell this list that a
  // reader had cleared filters they never saw.
  const expressed = seeded || mine.size > 0;
  return {
    q: mine.get("q") ?? "",
    // No sort in an EXPRESSED address means the server's own order, not the
    // screen's. The two are different lists — a saved view that names no sort
    // asks for the former — and absence can only spell one of them, which is
    // why the writer below spells a sort out even when it is the default one.
    sort: mine.get("sort") ?? (expressed ? "" : opening.sort),
    includeArchived: mine.get("include_archived") === "true",
    filters: expressed ? filters : opening.filters,
    perPage: pageSizeOf(mine.get(PER_PAGE_PARAM)) ?? opening.perPage,
  };
}

/**
 * The address `query` produces: every dial that is NOT at its default.
 *
 * A default left out is what keeps a shared link about what the reader chose.
 * `#/companies?q=acme` says one thing; the same address carrying the page size
 * and `include_archived=false` says the same thing while hiding it, and invites
 * the next reader to think three dials were turned.
 *
 * SORT is the exception, and is written even when it is the screen's own: an
 * unsorted list is a state a reader can reach — a saved view that names no
 * sort asks for the server's order rather than this screen's — and absence
 * cannot spell both that and "whatever this list opens in". So absence is kept
 * for the one that has no other spelling, and the ordinary sort says its name.
 */
export function paramsFromListQuery(
  query: ListQuery,
  opening: ListQuery,
  /** The address prefix this list's dials carry; see `scoped`. */
  scope?: string,
): UrlParams {
  const params = new Map<string, string>();
  const set = (name: string, value: string) =>
    params.set(scoped(scope, name), value);
  for (const [key, value] of Object.entries(query.filters)) {
    set(key, value);
  }
  if (query.q) {
    set("q", query.q);
  }
  if (query.sort) {
    set("sort", query.sort);
  }
  if (query.includeArchived) {
    set("include_archived", "true");
  }
  if (query.perPage !== opening.perPage) {
    set(PER_PAGE_PARAM, String(query.perPage));
  }
  return params;
}

/** The most rows one list read may ask for (the contract's CAP-PAGE ceiling). */
const MAX_ROWS_PER_READ = 200;

/**
 * How many rows to fetch for a footer page size of `perPage`: as many WHOLE
 * rendered pages as one read is allowed to carry.
 *
 * A keyset cursor can only walk forward one read at a time, so a pager can
 * only number the pages already in hand. Reading a page at a time therefore
 * numbers exactly one, and every further number has to be earned by a round
 * trip — which is what made a second page appear only after the reader had
 * been told none existed. Reading in whole multiples instead puts several
 * numbered pages in hand at once, each of them a slice of ONE response, so
 * they are also one consistent snapshot: no row shows up twice and none is
 * skipped between them.
 *
 * Whole multiples matter. A remainder would leave a last page shorter than the
 * size the footer names, and asking for more than the ceiling gets silently
 * clamped, leaving the pager offering numbers the response has no rows for.
 */
export function listFetchLimit(perPage: number): number {
  return Math.max(1, Math.floor(MAX_ROWS_PER_READ / perPage)) * perPage;
}

export type ListPage<Row> = {
  data: Row[];
  page: { next_cursor: string | null; has_more: boolean };
};

/**
 * One value a filter chip offers: named by a message key when the vocabulary
 * is the screen's own, or by the server's text when it is an administered list
 * (lead sources, for one) the screen only relays.
 */
export type FilterOption =
  | { value: string; label: MessageKey }
  | { value: string; text: string };

/** A filter chip, declared in the screen's own message keys. */
export type FilterSpec = {
  key: string;
  label: MessageKey;
  /** The "no filter" entry, which is also how a chosen value is cleared. */
  allLabel: MessageKey;
  options: FilterOption[];
};

/** A screen's own preset: a named sort + filter preset, shown as a tab. */
export type ViewSpec = {
  label: MessageKey;
  sort?: string;
  filters?: Record<string, string>;
};

/**
 * The reader's own saved view, as a tab: the WHOLE list state it claims, plus
 * the id that says which view it is.
 *
 * Both halves are load-bearing. A tab carrying only the sort and the filters
 * restores a view saved with "Show archived" on as a view with it off — and
 * then the tab does not even match itself, because the archived toggle is part
 * of what a view claims about the list. The search is here for the same reason:
 * it narrows the list exactly as a filter does, so a tab that omitted it would
 * light while a search nobody saved was narrowing the rows underneath it. And
 * the id is what survives a rename or a newly created view: `/views` answers
 * ordered by name, so a position is a different view as soon as one is inserted
 * ahead of it.
 *
 * `perPage` is optional because a stored blob need not claim one: a view that
 * saved a page size asks for it, and a view that saved none leaves the reader's
 * own choice alone.
 */
export type SavedViewTab = Readonly<{
  id: string;
  label: string;
  q: string;
  sort: string;
  filters: Readonly<Record<string, string>>;
  includeArchived: boolean;
  perPage?: number;
}>;

/**
 * One tab on the rail, whichever kind it came from: a screen's preset or a
 * reader's saved view. `perPage` is optional because neither kind need claim a
 * page size — a preset never does, and a saved view only when its stored blob
 * carried one.
 */
type RailView = ListView &
  Readonly<{
    id: string;
    q: string;
    includeArchived: boolean;
    perPage?: number;
  }>;

export function useListQuery<Row>({
  key,
  fetchPage,
  initialSort,
  initialFilters,
  screenDials = NO_SCREEN_DIALS,
  paramScope,
}: Readonly<{
  key: string;
  fetchPage: (
    query: ListQuery,
    cursor: string | null,
  ) => Promise<ListPage<Row>>;
  initialSort?: string;
  /**
   * Filters the list opens on, for a reader who has not narrowed it themselves.
   *
   * Read at ARRIVAL only. The opening state is written into the address once, on
   * mount, and from then on a bare address means the reader cleared everything
   * rather than that they have just got here — so a filter that changes after
   * that seeding does not reach the list. Every caller today passes a value that
   * is already resolved by the time this mounts (the leads queue takes the
   * viewer's id from behind a QueryGate); a caller that cannot must gate its own
   * mount the same way rather than expect a late value to be picked up.
   */
  initialFilters?: Readonly<Record<string, string>>;
  /**
   * Address keys this SCREEN owns, which are not dials of the list.
   *
   * A drawing choice — board or table — belongs in the address and does not
   * belong on the wire. Naming it here keeps the codec from reading it as a
   * filter; see `withoutScreenDials`.
   *
   * A STABLE reference — a module-level constant, as the callers pass — because
   * the query is memoised on it; a fresh array each render would rebuild the
   * query, and with it the react-query key, on every one.
   */
  screenDials?: readonly string[];
  /**
   * The address prefix this list's dials carry, for a route that holds MORE
   * THAN ONE list — the settings Data-model tab draws the products table and
   * the offer-template table together, and one flat parameter space described
   * both at once. Omit it everywhere else: a scope on a list that owns its
   * address alone only makes the link people paste uglier. See `scoped`.
   */
  paramScope?: string;
}>) {
  // In overlay mode the incumbent mirror refuses sort/filter dials (422), so
  // list reads must carry neither: an empty sort (ListTable hides the controls
  // to match) and no filters. Native mode keeps the screen's default sort.
  const overlay = useSorMode() === "overlay";
  const [params, setParams] = useUrlParams();
  const opening = useMemo<ListQuery>(
    () => ({
      q: "",
      sort: overlay ? "" : (initialSort ?? ""),
      includeArchived: false,
      // Overlay withholds filters for the same reason it withholds sort: the
      // incumbent mirror answers 422 to both. A screen that opens on a narrowed
      // list opens unnarrowed there rather than sending a dial the mirror
      // refuses.
      filters: overlay ? {} : (initialFilters ?? {}),
      perPage: LIST_PAGE_SIZES[0],
    }),
    [overlay, initialSort, initialFilters],
  );

  // Whether the list's opening state has been spelled into the address yet.
  //
  // A bare address has two meanings nothing in it can tell apart: "I have just
  // arrived" and "I cleared everything". Rather than carry a flag that decides
  // between them — which changes what an unchanged address MEANS, and so can
  // move without anything re-rendering — the opening state is WRITTEN to the
  // address on arrival. After that a bare address has one meaning: nothing is
  // set, because the reader unset it.
  const seeded = useRef(false);

  // The query is DERIVED from the address rather than mirrored into state
  // beside it. Two copies would need an effect to reconcile them, and that
  // effect is where Back breaks: the address moves first, the state follows a
  // frame later, and whichever the list reads in between is the wrong one.
  // Derived, Back is just another address, and there is nothing to reconcile.
  //
  // Overlay reads the address for nothing, because the mirror refuses every
  // dial in it. A pasted link therefore opens an unnarrowed list there instead
  // of a 422 the reader cannot act on.
  const query = useMemo(
    () =>
      overlay
        ? opening
        : listQueryFromParams(
            withoutScreenDials(params, screenDials),
            opening,
            seeded.current,
            paramScope,
          ),
    [overlay, params, opening, screenDials, paramScope],
  );

  // Spell the opening state into the address, once, on arrival.
  //
  // It runs before the reader can turn anything (an effect on mount), and the
  // guard is what keeps it from putting a filter back after they cleared the
  // last one. A screen whose opening state is already the default writes
  // nothing, and needs to: for that screen a bare address never meant anything
  // else.
  useEffect(() => {
    if (seeded.current || overlay) {
      return;
    }
    seeded.current = true;
    if (ownDials(currentParams(), paramScope).size > 0) {
      // The address already says what the list should be — a reload, a link
      // somebody was sent, or Back onto a list this mount did not narrow.
      // Writing the opening state over it would throw away exactly what was
      // asked for, and a reload is where that is most obvious: the reader
      // watches their own filter vanish.
      return;
    }
    setParams(
      replaceOwn(
        currentParams(),
        paramsFromListQuery(opening, opening, paramScope),
        paramScope,
      ),
    );
  }, [overlay, opening, setParams, paramScope]);

  const setQuery = useCallback(
    (update: SetStateAction<ListQuery>) => {
      if (overlay) {
        return;
      }
      const live = currentParams();
      const next =
        typeof update === "function"
          ? // Read from the LIVE address, never from this render's snapshot: a
            // debounced write must not carry a query from before the sort the
            // reader changed while it was settling, and two writes in one
            // handler — pressing a saved view restores four dials — must
            // compose rather than the last one landing alone.
            update(
              listQueryFromParams(
                withoutScreenDials(currentParams(), screenDials),
                opening,
                seeded.current,
                paramScope,
              ),
            )
          : update;
      // The rendered page is the list's own dial but not one this codec
      // computes, so it is carried across rather than dropped: without this,
      // the first write of any kind wipes the page out of an address a reader
      // was sent to. A write that really is a NARROWING still resets it — the
      // table's own reset fires straight after and takes it back to one.
      setParams(
        mergeScreenDials(
          replaceOwn(
            live,
            withListPage(
              paramsFromListQuery(next, opening, paramScope),
              listPageOf(live, paramScope),
              paramScope,
            ),
            paramScope,
          ),
          live,
          screenDials,
        ),
      );
    },
    [overlay, opening, setParams, screenDials, paramScope],
  );

  const infinite = useInfiniteQuery({
    queryKey: [key, query],
    queryFn: ({ pageParam }) => fetchPage(query, pageParam),
    initialPageParam: FIRST_PAGE,
    getNextPageParam: (last) =>
      last.page.has_more && last.page.next_cursor
        ? last.page.next_cursor
        : undefined,
  });
  const rows = (infinite.data?.pages ?? []).flatMap((page) => page.data);
  return {
    rows,
    query,
    setQuery,
    paramScope,
    hasMore: infinite.hasNextPage,
    loadMore: () => infinite.fetchNextPage(),
    isPending: infinite.isPending,
    isError: infinite.isError,
    error: infinite.error,
    refetch: () => infinite.refetch(),
  };
}

export type ListState<Row> = Readonly<{
  rows: Row[];
  query: ListQuery;
  setQuery: Dispatch<SetStateAction<ListQuery>>;
  /**
   * The address prefix this list's dials carry, so the surface addresses the
   * rendered page under the same name the rest of the dials go by. Undefined for
   * a list that owns its address alone, which is most of them.
   */
  paramScope?: string;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  hasMore: boolean;
  loadMore: () => void;
}>;

/**
 * The list surface bound to the server query: the state ladder every screen
 * renders identically (skeletons while pending, an error with a retry,
 * otherwise the table), with search, sort and filters reported straight back
 * into the ListQuery so the server answers them.
 *
 * The empty case belongs to the table rather than to this ladder: the table
 * knows whether the list is empty because nothing exists yet or because the
 * filters excluded everything, and only the second one should offer to clear
 * them.
 */
export function ListTable<Row>({
  state,
  columns,
  rowKey,
  rowRoute,
  unit,
  chips = [],
  dataChips = [],
  views = [],
  dataViews = [],
  action,
  caption,
  footer,
  searchable = true,
  showArchivedToggle = true,
  tools,
  emptyNote,
  scopeKey,
  body,
  bodyOwnsPaging = false,
  selection,
}: Readonly<{
  state: ListState<Row>;
  columns: readonly ListColumn<Row>[];
  rowKey: (row: Row) => string;
  /** Row selection for a bulk action — see the design-system ListTable. */
  selection?: ListSelection<Row>;
  /**
   * A second view of the same query, rendered instead of the grid while the
   * surface keeps its search, chips, views and page-size dial — see the
   * design-system ListTable's own `body`.
   */
  body?: ReactNode;
  /** The alternate body renders its own complete-count/load-more contract. */
  bodyOwnsPaging?: boolean;
  /**
   * Where a row's record lives. One declaration drives both ways in: clicking
   * the row navigates, and the identity cell becomes a real link that opens in
   * a new tab. Declaring them separately is how the two drift apart.
   */
  rowRoute?: (row: Row) => Route;
  /** Message key for the plural noun in the count and the empty state. */
  unit: MessageKey;
  chips?: readonly FilterSpec[];
  /**
   * Chips whose options are runtime record names rather than message keys — the
   * stages of a pipeline, the companies on the workspace. Already translated by
   * definition, since the server is what named them.
   */
  dataChips?: readonly ListChip[];
  views?: readonly ViewSpec[];
  /**
   * View tabs whose labels are server strings rather than message keys — the
   * reader's own saved views. Rendered after the screen's built-in presets, so
   * All/Mine/A-Z keep their positions as a saved view is added or removed.
   */
  dataViews?: readonly SavedViewTab[];
  action?: ReactNode;
  /** What this list is, for the lists that need saying. Never the screen name. */
  caption?: MessageKey;
  footer?: ReactNode;
  /** False for a list whose GET has no `q` param, e.g. /partners. */
  searchable?: boolean;
  showArchivedToggle?: boolean;
  /** Passed straight through to the surface's own tools slot, alongside the
   * Columns and Compact buttons — for the one screen (deals) whose board and
   * table views share a pipeline picker that lives beside them. */
  tools?: ReactNode;
  /**
   * What the empty table says under its generic line when THIS screen knows
   * why it is empty — a "Mine" view for a reader who owns nothing, with the
   * way back to everything. Overlay's own note wins when both apply.
   */
  emptyNote?: ReactNode;
  /**
   * What this list is reading, when the screen narrows it by something that is
   * neither a chip nor a filter — passed straight to the surface, which resets
   * the reader to page 1 when it changes. Deals is the case: its pipeline
   * picker is screen state, so switching pipelines leaves `filters` untouched
   * while the result set changes entirely.
   */
  scopeKey?: string;
}>): ReactNode {
  const t = useT();
  // Overlay reads a mirror that cannot sort or filter (the server 422s those
  // dials), so the table is handed neither, and a note says why. Search and
  // the archived toggle survive: the mirror answers the first and holds no
  // archived rows, so the second is a harmless no-op.
  const overlay = useSorMode() === "overlay";
  const { rows, query, setQuery, isPending, isError, error, refetch } = state;
  // The rendered page is a dial like the rest, so it comes from the address.
  // Read here rather than threaded through ListState: it is the only one the
  // TABLE owns by default, and a screen that never puts it in the URL keeps the
  // table's own number.
  const [params, setParams] = useUrlParams();
  // Where the reader was in the ROWS, which the shell cannot remember for them.
  // A full-height list takes the overflow itself, so the page column the shell
  // watches is at zero on every one of these screens and Back returned readers
  // to the top of a list they had scrolled a long way down. Its own lane, so a
  // record page's column and a list's rows are two separate places to be.
  const rowsScroller = useRef<HTMLDivElement>(null);
  useScrollMemory(rowsScroller, useHash(), "rows");
  const [localSearch, setLocalSearch] = useState(query.q);
  // What this box last put on the wire, so a `q` that moved for any OTHER
  // reason can be told apart from this box's own debounce landing.
  const committed = useRef(query.q);
  // Which view tab is lit is READ from the query rather than remembered: a tab
  // is a claim about what the list is showing, and a reader who then edits a
  // filter or a sort is no longer looking at that preset. Stored, the highlight
  // would keep claiming a view the query had already left; derived, it simply
  // stops matching, and comes back by itself if the reader undoes the edit.
  //
  // ONE rail, built once: the tabs the surface renders and the tabs the
  // highlight is matched against have to be the same list, or an index reported
  // by a press names a different view than the one that was pressed. A view tab
  // whose preset the mirror would refuse is a tab that lights up and does
  // nothing, so overlay mode has no rail at all — the same reason its chips and
  // its sort are withheld.
  const railViews: readonly RailView[] = overlay
    ? []
    : [
        ...views.map((spec) => ({
          ...translateView(spec, t),
          // A preset is identified by its message key: the screen wrote the set,
          // so the key is unique within it, and it stays the same string when
          // the label is translated or the rail is reordered. `preset:` keeps
          // it out of the uuid namespace a saved view's id lives in.
          id: `preset:${spec.label}`,
          // A preset is a sort and a set of filters over the whole live list:
          // it asks for no search and no archive, and the tab has to say so or
          // it would keep claiming the list while the reader has a search typed
          // or the archive switched on.
          q: "",
          includeArchived: false,
        })),
        ...dataViews,
      ];
  const [view, pickView] = useActiveView(railViews, query);
  // Keyed on the VALUES, not on the arrays that carry them. The table treats a
  // new `chosen` object as the reader narrowing the list and resets to page 1,
  // so an object rebuilt every render resets on every render: pressing Next
  // re-read page 2 and then immediately snapped back to page 1, and the list
  // flipped between the first two pages forever.
  //
  // `chips` cannot be the key — screens declare it as an inline array literal,
  // which is a fresh reference each render. The option values are what
  // chosenFor actually reads, and they are strings.
  // EVERY chip on the surface, declared and data-driven alike. The owner dial
  // is a dataChip because it names the viewer's teams at runtime, and code that
  // asks "which chips are there" must see it: reading `chips` alone is what
  // made picking Unassigned send `owner=unassigned:true` — the chip's key with
  // the raw option value — instead of `unassigned=true`, so the server ignored
  // an unknown parameter and answered the whole list.
  const allChips: readonly ListChip[] = [
    ...chips.map((chip) => translateChip(chip, t)),
    ...dataChips,
  ];
  // Keyed on the FILTERS, which are what the list actually reads. The table
  // treats a new `chosen` identity as the reader narrowing the list and resets
  // to page 1, so this identity must change only when the answer changes.
  //
  // Keying on the chip options made the roster do it: the owner dial names the
  // viewer's teams, which arrive on their own query, so a chip gained an option
  // seconds after the list rendered and threw the reader from page 2 back to
  // page 1 — for a reason they could not see, having touched nothing.
  //
  // Keying on chosenFor's RESULT is not enough either, and the saved views are
  // why. A restored view sets `owner_team_id=t-9` before the roster answers, so
  // the filter is already there when the matching option arrives; chosenFor
  // then adds the chip's own key and the result changes shape while the query,
  // and every row, stays exactly as it was.
  //
  // The filters are the honest signal: the fetch reads them and nothing else,
  // so an identity that follows them resets when the rows change and never
  // otherwise. What a dial OFFERS is presentation, and presentation must not
  // move the reader.
  // Sorted, so re-picking the same value cannot reorder the object and read as
  // a change: setFilter deletes a composite param and re-adds it, which moves
  // its insertion order without moving what it selects.
  const narrowKey = JSON.stringify(
    Object.entries(query.filters).sort(([a], [b]) => stable(a, b)),
  );
  // Rebuilt every render, on purpose: a dial must show what is chosen the
  // moment its options arrive. Nothing resets on this — `narrowKey` is the
  // trigger — so a fresh object here costs nothing.
  const chosen = chosenFor(allChips, query.filters);

  // A functional updater reads the query at commit time, not at the time the
  // timer was scheduled: a concurrent sort/filter/includeArchived change
  // (which sets query immediately, before this timer fires) is preserved
  // instead of being reverted by a stale closure over `query`. Skipped when
  // the screen isn't searchable — there is no debounce to race in that case.
  // The address moves without this screen unmounting — Back, Forward, a link to
  // the same list narrowed differently — and the box has to follow it or it
  // shows words the rows are not answering. Pressing Back out of a search left
  // the typed word sitting in a box over the unnarrowed list it had just left.
  //
  // Only for a `q` this box did not commit itself: the ref carries what it last
  // put on the wire, so a reader mid-word is never overwritten by their own
  // debounce arriving.
  useEffect(() => {
    if (query.q !== committed.current) {
      committed.current = query.q;
      setLocalSearch(query.q);
    }
  }, [query.q]);

  // The reader LEFT, with a word still settling.
  //
  // The effect above follows `q`, and a word typed against a list the reader
  // then walked away from has a `q` that never moved: type "acme", press Back
  // to a differently sorted view of the same list, and both addresses carry no
  // `q` at all. Nothing above fires, the timer keeps its appointment, and the
  // word lands on a list it was not typed into.
  //
  // `hashchange` is the discriminator and it is exact: `replaceParams` writes
  // with `history.replaceState`, which fires no such event and announces itself
  // in-app instead, so every dial this surface turns is silent here. What
  // reaches this listener is the browser's own navigation — Back, Forward, a
  // pasted link — which is precisely the case where a pending word belongs to a
  // list that is gone. Re-reading the address also re-arms the debounce with
  // the value now in it, so the stale timer commits nothing.
  useEffect(() => {
    const abandonPendingSearch = () => {
      const live = ownDials(currentParams(), state.paramScope).get("q") ?? "";
      committed.current = live;
      setLocalSearch(live);
    };
    globalThis.addEventListener("hashchange", abandonPendingSearch);
    return () =>
      globalThis.removeEventListener("hashchange", abandonPendingSearch);
  }, [state.paramScope]);

  useEffect(() => {
    if (!searchable) {
      return;
    }
    const timer = setTimeout(() => {
      committed.current = localSearch;
      setQuery((prev) =>
        prev.q === localSearch ? prev : { ...prev, q: localSearch },
      );
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [localSearch, setQuery, searchable]);

  // Neither state replaces the surface: the header, the dials and the primary
  // action belong to the screen rather than to the response, so they stay put
  // and only the body reports what happened.
  const problem = isError ? (
    <>
      <p>{t("common.error")}</p>
      <p className="t-mono" style={{ marginTop: "var(--space-1)" }}>
        {problemMessageOf(error, t)}
      </p>
      <Button
        small
        onClick={() => refetch()}
        style={{ marginTop: "var(--space-2)" }}
      >
        {t("common.retry")}
      </Button>
    </>
  ) : undefined;

  // Pressing a tab pins it AND restores the list state it holds. The surface
  // rewrites the sort and the filters itself; the archived toggle and the page
  // size are this layer's, because they are part of the ListQuery the fetchers
  // read and the surface knows nothing about a saved view's stored state.
  //
  // The toggle is rewritten unconditionally, exactly as the filters are: a view
  // describes the WHOLE list, so an archived toggle left on from the previous
  // tab both widens the view the reader just picked and stops that tab matching
  // itself. Page size is the one dial a tab may make no claim about — a preset
  // never claims one, and a saved view only where its stored blob carried one —
  // so the reader's own choice survives a tab that carries none.
  const applyViewState = (index: number) => {
    pickView(index);
    const picked = railViews[index];
    if (!picked) {
      return;
    }
    setQuery((prev) => ({
      ...prev,
      includeArchived: picked.includeArchived,
      perPage: picked.perPage ?? prev.perPage,
    }));
  };

  const setFilter = (key: string, value: string) =>
    setQuery((prev) => {
      const filters = { ...prev.filters };
      // A chip whose options each name a DIFFERENT query parameter carries the
      // parameter in the value, as `param:value` (the owner dial: mine, my
      // team's, unowned — one question the server answers three ways, and
      // refuses if asked two at once). Clearing such a chip has to drop
      // whichever parameter is currently set, not the chip's own key, which
      // names no parameter at all.
      // Only THIS chip's own parameters are cleared. A chip whose options each
      // name a different query parameter (the owner dial: mine, my team's,
      // unowned) has to drop whichever of its own it currently holds, because
      // its key names no parameter at all. Clearing every composite parameter
      // on the surface instead would make picking a lifecycle silently drop the
      // owner filter — one dial reaching into another's answer.
      const mine = allChips
        .filter((chip) => chip.key === key)
        .flatMap((chip) => chip.options.map((option) => option.value))
        .filter((candidate) => candidate.includes(":"))
        .map((candidate) => candidate.slice(0, candidate.indexOf(":")));
      for (const param of mine) {
        delete filters[param];
      }
      if (value) {
        const split = value.indexOf(":");
        if (split > 0 && mine.includes(value.slice(0, split))) {
          filters[value.slice(0, split)] = value.slice(split + 1);
        } else {
          filters[key] = value;
        }
      } else {
        delete filters[key];
      }
      return { ...prev, filters };
    });

  return (
    <ListSurface<Row>
      rows={rows}
      columns={columns}
      rowKey={rowKey}
      onRowClick={rowRoute ? (row) => navigate(rowRoute(row)) : undefined}
      rowHref={rowRoute ? (row) => routeHash(rowRoute(row)) : undefined}
      unit={t(unit)}
      // An empty overlay list is far more often a mirror row whose HubSpot
      // owner email has no matching workspace user (so mirror_visibility never
      // grants it to anyone) than a genuinely empty HubSpot portal — name that
      // cause rather than letting the generic empty copy imply "there is
      // nothing here".
      //
      // Not while a search is narrowing it, though: then the reader's own words
      // are the likeliest cause, and blaming the mirror's owner mapping for
      // what a typo did would send them looking in the wrong place.
      emptyNote={
        overlay
          ? query.q
            ? undefined
            : t("overlay.emptyOwnerHint")
          : emptyNote
      }
      action={action}
      caption={caption ? t(caption) : undefined}
      footer={footer}
      tools={tools}
      note={overlay ? t("list.overlayReadOnly") : undefined}
      search={
        searchable
          ? { value: localSearch, onChange: setLocalSearch }
          : undefined
      }
      sort={
        overlay
          ? undefined
          : {
              value: query.sort,
              onChange: (next) => setQuery((prev) => ({ ...prev, sort: next })),
            }
      }
      chips={overlay ? [] : allChips}
      chosen={chosen}
      onChipChange={setFilter}
      views={railViews}
      activeView={view}
      narrowKey={narrowKey}
      scopeKey={scopeKey}
      onViewChange={applyViewState}
      archived={
        showArchivedToggle
          ? {
              checked: query.includeArchived,
              onChange: (next) =>
                setQuery((prev) => ({ ...prev, includeArchived: next })),
            }
          : undefined
      }
      // An overlay mirror pages by cursor like the native store, so paging is
      // the one dial that behaves identically in both modes.
      hasMore={state.hasMore}
      onLoadMore={state.loadMore}
      // The page size is part of the server query, not a second slice on top
      // of it: changing it re-asks the server, which is why it lives in the
      // ListQuery the fetchers read their `limit` from.
      perPage={query.perPage}
      onPerPage={(next) => setQuery((prev) => ({ ...prev, perPage: next }))}
      // The rendered page is in the address too, so paging through a list and
      // opening a record no longer costs the reader their place. Page one is
      // left OUT of it: it is where every list opens, and an address that said
      // so on arrival would put a dial in front of a reader who turned nothing.
      page={listPageOf(params, state.paramScope)}
      onPage={(next) =>
        setParams(withListPage(currentParams(), next, state.paramScope))
      }
      bodyRef={rowsScroller}
      // The unit key names the table for the widths it remembers.
      widthsKey={unit}
      body={body}
      bodyOwnsPaging={bodyOwnsPaging}
      selection={selection}
      pending={isPending}
      problem={problem}
    />
  );
}

type Translate = ReturnType<typeof useT>;

function translateChip(chip: FilterSpec, t: Translate): ListChip {
  return {
    key: chip.key,
    label: t(chip.label),
    allLabel: t(chip.allLabel),
    options: chip.options.map((option) => ({
      value: option.value,
      label: "text" in option ? option.text : t(option.label),
    })),
  };
}

function translateView(spec: ViewSpec, t: Translate): ListView {
  return { label: t(spec.label), sort: spec.sort, filters: spec.filters };
}

/**
 * Which view tab is lit, and the setter the table reports a press to.
 *
 * The highlight is DERIVED from the query, so a reader who edits a filter is no
 * longer claimed to be looking at the preset they started from. But two views
 * can ask for the same sort and filters — a saved "German customers" that
 * narrows exactly as the built-in Customers preset does — and a purely derived
 * highlight lights the first match, so the reader's own view never lights when
 * they pick it.
 *
 * The pressed tab therefore wins, but only while it still describes the list.
 * The moment the query moves away it stops matching and the highlight falls
 * back to whatever the query now describes.
 *
 * What is pinned is the view's ID, never its position. `/views` answers ordered
 * by name, so creating or renaming a view re-orders the rail under the reader:
 * a stored index then points at whichever view moved into that slot, and the
 * highlight silently claims a view nobody picked.
 */
function useActiveView(
  views: readonly RailView[],
  query: ListQuery,
): [number, (index: number) => void] {
  const [pickedId, setPickedId] = useState<string | null>(null);
  const matched = views.findIndex((spec) => matchesView(spec, query));
  const pinned = views.findIndex((spec) => spec.id === pickedId);
  const view =
    pinned >= 0 && matchesView(views[pinned], query) ? pinned : matched;
  // The press reports an index into the rail just rendered, which is where the
  // id it names comes from. A press on nothing (the surface's own "clear
  // everything" reports index 0 against an empty rail) unpins rather than
  // pinning a view that is not there.
  return [view, (index: number) => setPickedId(views[index]?.id ?? null)];
}

/**
 * Is the list showing exactly what this view asks for, nothing added or left?
 *
 * Takes the state a view describes rather than its name, so it reads a screen's
 * built-in preset and a reader's saved view the same way — the two differ only
 * in where the label came from, and the highlight is about the query.
 *
 * Every dial that decides WHICH rows the list holds counts, and the two that
 * are easy to leave out are the reason this is spelled out. The archived toggle
 * widens the list exactly as a filter narrows it, so a view saved with "Show
 * archived" on is a DIFFERENT view from the default one. The search narrows it
 * the same way: a view saved over "acme" describes the acme rows, and a tab that
 * ignored the box would light over the whole list and name it acme.
 *
 * The invariant, then: a tab is lit only while the list on screen is the one it
 * names, down to the search box. Page size is the exception it is allowed to be,
 * because it changes how many of those rows are drawn at once and not which rows
 * they are.
 */
function matchesView(
  spec: Readonly<{
    q?: string;
    sort?: string;
    filters?: Readonly<Record<string, string>>;
    includeArchived?: boolean;
  }>,
  query: ListQuery,
): boolean {
  if (query.q !== (spec.q ?? "")) {
    return false;
  }
  if (query.sort !== (spec.sort ?? "")) {
    return false;
  }
  if (query.includeArchived !== (spec.includeArchived ?? false)) {
    return false;
  }
  const wanted = Object.entries(spec.filters ?? {});
  const applied = Object.entries(query.filters).filter(([, value]) => value);
  return (
    wanted.length === applied.length &&
    wanted.every(([key, value]) => query.filters[key] === value)
  );
}

/**
 * The owner dials every record list offers: mine, my team's, and the unowned
 * queue.
 *
 * One chip rather than three, because they answer ONE question — whose rows —
 * and the server refuses two of them at once (they name different sets, so a
 * pair can only ever match nothing). A single-select chip makes that refusal
 * unreachable from the UI instead of something the reader discovers as a 422.
 *
 * Built only once /me has answered. A chip option whose value is still "" reads
 * as "clear this filter" to the table, so a half-built dial would quietly
 * narrow nothing while looking armed.
 *
 * The wire takes ONE team id, so a viewer on several teams gets one dial per
 * team rather than a guess about which of them was meant; `viewerTeamOptions`
 * builds those.
 */
/**
 * The tag dials: which word narrows this list, and how several combine.
 *
 * Two chips rather than one control, because they answer two questions and a
 * reader sets them at different moments — a word first, and the mode only when
 * a second word makes the question ambiguous. Both write into
 * `ListQuery.filters` under the wire's own names, so the address, the request
 * and the saved view all say the same thing.
 *
 * By ID, never by name. A name is what a person types and an admin can rename,
 * so a saved view holding one would quietly start selecting a different slice
 * the day somebody corrects a spelling.
 *
 * The mode chip is drawn whatever is selected. Hiding it until a second tag is
 * picked would make a dial appear under the reader's cursor as a side effect of
 * choosing a word, and its value is already in the address either way.
 */
export function useTagChips(): readonly ListChip[] {
  const t = useT();
  const vocabulary = useTagVocabulary();
  const words = vocabulary.data?.tags ?? [];
  if (words.length === 0) {
    // No vocabulary to filter by — or none this caller may read. Either way a
    // dial whose every option is "all" is a control that cannot answer.
    return [];
  }
  return [
    {
      key: "tag_id",
      label: t("tags.columnHeader"),
      allLabel: t("tags.filterAll"),
      options: words.map((tag) => ({ value: tag.id, label: tag.name })),
    },
    {
      key: "tag_mode",
      label: t("tags.filterModeLabel"),
      allLabel: t("tags.filterModeAny"),
      options: [
        { value: "all", label: t("tags.filterModeAll") },
        { value: "none", label: t("tags.filterModeNone") },
      ],
    },
  ];
}

export function useOwnerChips(): readonly ListChip[] {
  const t = useT();
  const me = useMe();
  const viewerId = me.data?.user.id;
  const teams = useRoster("team", Boolean(viewerId));
  const teamsPartial = useRosterPartial("team", Boolean(viewerId));
  if (!viewerId) {
    return [];
  }
  return [
    {
      key: "owner",
      label: t("list.owner"),
      allLabel: t("list.filterOwnerAll"),
      options: [
        { value: `owner_id:${viewerId}`, label: t("list.filterOwnerMe") },
        ...viewerTeamOptions(me.data?.teams ?? [], teams, teamsPartial, t),
        { value: "unassigned:true", label: t("list.filterOwnerUnassigned") },
      ],
    },
  ];
}

/**
 * One dial per team the viewer belongs to, named off the roster.
 *
 * Named, not counted. The viewer may sit in several teams, and a dial that
 * withheld itself past the first one left everyone on two teams unable to ask a
 * question the API answers. Each team is its own option, so picking one is
 * picking a team rather than accepting a guess about which was meant.
 *
 * The IDS come from /me and only the LABELS come from the roster, so the two
 * reads can disagree about which teams there are. Filtering the roster BY the
 * membership made the label's absence decide the dial's: a team the walk never
 * reached yielded no option at all, and the viewer silently lost a filter the
 * API would still have answered. So every membership gets its option, and a name
 * the roster could not supply is reported as missing rather than taken as proof
 * the team is.
 */
function viewerTeamOptions(
  memberships: readonly string[],
  roster: ReturnType<typeof useRoster>,
  partial: boolean,
  t: ReturnType<typeof useT>,
): { value: string; label: string }[] {
  const named = new Map(
    (roster.data ?? []).map((entry) => [
      entry.id,
      // The team roster: every entry carries a name. The user roster is the
      // other kind this hook serves, and it is never asked for here.
      "display_name" in entry ? entry.display_name : entry.name,
    ]),
  );
  const reading = rosterReading(roster, partial);
  return memberships.flatMap((teamId) => {
    const value = `owner_team_id:${teamId}`;
    const name = named.get(teamId);
    if (name) {
      return [{ value, label: name }];
    }
    // A roster walked to the END has answered about this team: it is not one
    // this reader may list — `/teams` excludes archived teams — and an option
    // whose only honest label is a uuid is worse than one dial fewer. The other
    // two readings have answered nothing about it, so the dial stays and says
    // which of the two it is waiting on.
    if (reading === "unnamed") {
      return [];
    }
    return [
      {
        value,
        label:
          reading === "pending" ? t("common.loading") : t("ref.nameLoadFailed"),
      },
    ];
  });
}

/**
 * What each chip currently shows, given the filters actually in the query.
 *
 * A normal chip stores its value under its own key and needs no translation. A
 * chip whose options each name a different query parameter (`param:value` —
 * the owner dial) stores the value under the PARAMETER, so its selected option
 * has to be read back from whichever parameter is set. Without this the dial
 * narrows the list correctly and then renders as "Any owner", which reads as a
 * filter that did not take.
 */
function chosenFor(
  // Takes the option VALUES, so it reads a declared chip and a data-driven one
  // the same way — the two differ only in where their labels came from, and a
  // reader of this function never looks at a label.
  chips: readonly ListChip[],
  filters: Readonly<Record<string, string>>,
): Record<string, string> {
  const chosen = { ...filters };
  for (const chip of chips) {
    for (const option of chip.options) {
      const split = option.value.indexOf(":");
      if (split <= 0) {
        continue;
      }
      const param = option.value.slice(0, split);
      if (filters[param] === option.value.slice(split + 1)) {
        chosen[chip.key] = option.value;
      }
    }
  }
  return chosen;
}
