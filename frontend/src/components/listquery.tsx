import { useInfiniteQuery } from "@tanstack/react-query";
import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useEffect,
  useMemo,
  useRef,
} from "react";
import { FIRST_PAGE } from "../api/client";
import type { ListChip, ListView } from "../design-system/listtable";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useTagVocabulary } from "../lib/tags.queries";
import { currentParams, useUrlParams } from "../lib/urlstate";
import "./listquery.css";

// The shared list foundation (P-14): every list screen sends the rich
// q/sort/cursor/include_archived/filter vocabulary instead of a flat
// limit:50, and paginates by keyset (never offset — the workspace's rows
// mutate under a live feed). useListQuery owns the react-query wiring;
// ListTable binds that query to the list surface, which owns the controls.
import {
  LIST_PAGE_SIZES,
  type ListQuery,
  listPageOf,
  listQueryFromParams,
  mergeScreenDials,
  NO_SCREEN_DIALS,
  ownDials,
  paramsFromListQuery,
  replaceOwn,
  withListPage,
  withoutScreenDials,
} from "./listqueryparams";

export {
  LIST_PAGE_SIZES,
  type ListQuery,
  listFetchLimit,
  listPageOf,
  listQueryFromParams,
  mergeScreenDials,
  paramsFromListQuery,
  SEARCH_DEBOUNCE_MS,
  withListPage,
  withoutScreenDials,
} from "./listqueryparams";
export { ListTable } from "./listquerytable";

export type ListPage<Row> = {
  data: Row[];
  page: {
    next_cursor: string | null;
    has_more: boolean;
    /**
     * How many rows match in total, when the endpoint counts. Undefined means
     * it does not count — never that it counted zero — so a list reading this
     * falls back to how many rows it has loaded rather than reporting empty.
     */
    total?: number;
  };
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
export type RailView = ListView &
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
  enabled,
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
   * address alone only makes the link contacts paste uglier. See `scoped`.
   */
  paramScope?: string;
  /** Whether the list may be asked for this query; it is not, while false. */
  enabled?: (query: ListQuery) => boolean;
}>) {
  const [params, setParams] = useUrlParams();
  const opening = useMemo<ListQuery>(
    () => ({
      q: "",
      sort: initialSort ?? "",
      includeArchived: false,
      filters: initialFilters ?? {},
      perPage: LIST_PAGE_SIZES[0],
    }),
    [initialSort, initialFilters],
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
  const query = useMemo(
    () =>
      listQueryFromParams(
        withoutScreenDials(params, screenDials),
        opening,
        seeded.current,
        paramScope,
      ),
    [params, opening, screenDials, paramScope],
  );

  // Spell the opening state into the address, once, on arrival.
  //
  // It runs before the reader can turn anything (an effect on mount), and the
  // guard is what keeps it from putting a filter back after they cleared the
  // last one. A screen whose opening state is already the default writes
  // nothing, and needs to: for that screen a bare address never meant anything
  // else.
  useEffect(() => {
    if (seeded.current) {
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
  }, [opening, setParams, paramScope]);

  const setQuery = useCallback(
    (update: SetStateAction<ListQuery>) => {
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
    [opening, setParams, screenDials, paramScope],
  );

  const infinite = useInfiniteQuery({
    queryKey: [key, query],
    enabled: enabled?.(query) ?? true,
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
    // Read off the LAST page fetched rather than the first: each response
    // recounts against the same filters, so the newest answer is the one that
    // has seen the most recent writes.
    total: infinite.data?.pages.at(-1)?.page.total,
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
  /**
   * How many rows match the current filters on the server, when it says. The
   * rendered count falls back to the rows in hand where it does not.
   */
  total?: number;
}>;

/**
 * The tag dial: which word narrows this list.
 *
 * ONE word, not several. This surface holds one value per filter key — a chip
 * whose row is already applied is dropped from the "add filter" menu, and
 * `setFilter` assigns rather than appends — so a second tag would replace the
 * first, not join it. The endpoint takes several ids and a mode that combines
 * them, and the encoder in `tagfilter.ts` speaks that shape; what is missing is
 * a surface that can hold two values, which is a change to the filter bar
 * rather than to tags. Offering an any/all/none dial over a filter that can
 * only ever hold one word would be a control that cannot do what it says.
 *
 * By ID, never by name. A name is what a human types and an admin can rename,
 * so a saved view holding one would quietly start selecting a different slice
 * the day somebody corrects a spelling.
 */
export function useTagChips(): readonly ListChip[] {
  const t = useT();
  const vocabulary = useTagVocabulary();
  // Whether a tag is narrowing this list RIGHT NOW, read from the address
  // rather than passed in: the address is where an applied filter lives on
  // every one of these screens, and a caller that forgot the argument would
  // get the dropped dial silently.
  // Any scope's, not just the unscoped one: a list drawn beside another carries
  // its dials under a `<scope>.` prefix, and a bare lookup would drop the chip
  // on exactly the screens that have two lists.
  const [params] = useUrlParams();
  const applied = [...params.keys()].some(
    (key) => (key === "tag_id" || key.endsWith(".tag_id")) && params.get(key),
  );
  const words = vocabulary.data?.tags ?? [];
  // A dial with no options is dropped ONLY when nothing is filtering. An empty
  // list here is three different answers — still loading, a workspace with no
  // words, and a caller who may not read the vocabulary — and the request
  // carries the applied id whichever it is. Dropping the row then leaves the
  // list narrowed by a filter with no control to clear it, which is a reader
  // staring at an empty page with nothing on screen explaining why.
  if (words.length === 0 && !applied) {
    return [];
  }
  return [
    {
      key: "tag_id",
      // The label says the list is short when the catalog was cut. The
      // vocabulary read is capped and has no cursor, so past the cap a word
      // that exists is indistinguishable from one that does not — and a reader
      // who cannot find a word in a dial concludes the workspace lacks it.
      label: vocabulary.data?.truncated
        ? t("tags.columnHeaderPartial")
        : t("tags.columnHeader"),
      allLabel: t("tags.filterAll"),
      options: words.map((tag) => ({ value: tag.id, label: tag.name })),
    },
  ];
}
