import { type ReactNode, useEffect, useRef, useState } from "react";
import { Button } from "../design-system/atoms";
import {
  type ListChip,
  type ListColumn,
  type ListSelection,
  ListTable as ListSurface,
} from "../design-system/listtable";
import { stable } from "../format/collate";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "../lib/problem";
import { navigate, type Route, routeHash, useHash } from "../lib/router";
import { useScrollMemory } from "../lib/scrollmemory";
import { currentParams, useUrlParams } from "../lib/urlstate";
import type {
  FilterSpec,
  ListState,
  RailView,
  SavedViewTab,
  ViewSpec,
} from "./listquery";
import {
  listPageOf,
  ownDials,
  SEARCH_DEBOUNCE_MS,
  withListPage,
} from "./listqueryparams";
import {
  chosenFor,
  pickedFilters,
  translateChip,
  translateView,
  useActiveView,
} from "./listqueryviews";

/**
 * The list surface bound to the server query. Every screen renders the same
 * state ladder: skeletons while pending, an error with a retry, otherwise the
 * table. Search, sort and filters go back into the ListQuery, so the server
 * answers them.
 *
 * The empty case belongs to the table, not this ladder. The table knows if the
 * list is empty because nothing exists yet or because the filters excluded
 * everything. Only the second case should offer to clear them.
 */
export function ListTable<Row>({
  title,
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
  saveView,
  emptyNote,
  scopeKey,
  body,
  bodyOwnsPaging = false,
  bodyCount,
  selection,
  hideShortChrome,
}: Readonly<{
  /**
   * The page's own name, for a screen where this table is the page. Handed
   * through to the design-system surface, which documents the obligation that
   * comes with it: the screen also joins `SELF_HEADED_SCREENS`.
   */
  title?: ReactNode;
  state: ListState<Row>;
  columns: readonly ListColumn<Row>[];
  rowKey: (row: Row) => string;
  /** Row selection for a bulk action; see the design-system ListTable. */
  selection?: ListSelection<Row>;
  /**
   * A second view of the same query, rendered instead of the grid. The
   * surface keeps its search, chips, views and page-size dial. See the
   * design-system ListTable's own `body`.
   */
  body?: ReactNode;
  /** The alternate body renders its own complete-count/load-more contract. */
  bodyOwnsPaging?: boolean;
  /** The count for a body that pages itself; see the design-system table. */
  bodyCount?: ReactNode;
  /** See the design-system table's own `hideShortChrome`. */
  hideShortChrome?: boolean;
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
   * Chips whose options are runtime record names rather than message keys:
   * the stages of a pipeline, the companies on the workspace. Already
   * translated by definition, since the server is what named them.
   */
  dataChips?: readonly ListChip[];
  views?: readonly ViewSpec[];
  /**
   * View tabs whose labels are server strings rather than message keys: the
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
  /** Passed through to the surface's own tools slot, after the Display menu.
   * For a screen whose board and table views share a dial that lives beside
   * them, e.g. deals' pipeline picker. */
  tools?: ReactNode;
  /** The screen's Save view, in the surface's own last slot; see
   * `ListSurface`. A slot rather than the tail of `tools`, so the row's order
   * is the surface's to keep and not each screen's to remember. */
  saveView?: ReactNode;
  /**
   * What the empty table says under its generic line, when this screen knows
   * why it is empty. Example: a "Mine" view for a reader who owns nothing,
   * with the way back to everything.
   */
  emptyNote?: ReactNode;
  /**
   * What this list is reading, when the screen narrows it by something that
   * is neither a chip nor a filter. Passed to the surface, which resets the
   * reader to page 1 when it changes. Deals is the case: its pipeline picker
   * is screen state. So switching pipelines leaves `filters` untouched while
   * the result set changes entirely.
   */
  scopeKey?: string;
}>): ReactNode {
  const t = useT();
  const { rows, query, setQuery, isPending, isError, error, refetch } = state;
  // The rendered page is a dial like the rest, so it comes from the address.
  // Read here rather than threaded through ListState: it is the only dial the
  // table owns by default. A screen that never puts it in the URL keeps the
  // table's own number.
  const [params, setParams] = useUrlParams();
  // Where the reader was in the rows, which the shell cannot remember for them.
  // A full-height list takes the overflow itself, so the page column the shell
  // watches is at zero on every one of these screens. Back then returned
  // readers to the top of a list they had scrolled a long way down. Its own
  // lane, so a record page's column and a list's rows are two separate places.
  const rowsScroller = useRef<HTMLDivElement>(null);
  useScrollMemory(rowsScroller, useHash(), "rows");
  const [localSearch, setLocalSearch] = useState(query.q);
  // What this box last put on the wire. It tells a `q` that moved for another
  // reason apart from this box's own debounce landing.
  const committed = useRef(query.q);
  // The box's word as of the last write to it, ahead of any render. The timer
  // reads this when it fires, not the word it was armed with. Back reaches this
  // ref at `popstate`; the render that cancels the timer comes a task later,
  // and a loaded browser can run the timer first.
  const typed = useRef(query.q);
  // Which view tab is lit is read from the query, not remembered. A tab claims
  // what the list is showing. A reader who then edits a filter or a sort is no
  // longer looking at that preset. Stored, the highlight would keep claiming a
  // view the query had left. Derived, it stops matching, and comes back by
  // itself if the reader undoes the edit.
  //
  // One rail, built once. The tabs the surface renders and the tabs the
  // highlight is matched against must be the same list. Otherwise an index
  // reported by a press names a different view than the one pressed.
  const railViews: readonly RailView[] = [
    ...views.map((spec) => ({
      ...translateView(spec, t),
      // A preset is identified by its message key. The screen wrote the set,
      // so the key is unique within it. It stays the same string when the
      // label is translated or the rail is reordered. `preset:` keeps it out
      // of the uuid namespace a saved view's id lives in.
      id: `preset:${spec.label}`,
      // A preset is a sort and a set of filters over the whole live list. It
      // asks for no search and no archive, and the tab has to say so. If not,
      // it would keep claiming the list while the reader has a search typed
      // or the archive switched on.
      q: "",
      includeArchived: false,
    })),
    ...dataViews,
  ];
  const [view, pickView] = useActiveView(railViews, query);
  // Keyed on the values, not on the arrays that carry them. The table treats a
  // new `chosen` object as the reader narrowing the list and resets to page 1.
  // So an object rebuilt every render resets on every render. Pressing Next
  // re-read page 2, then snapped back to page 1, and the list flipped between
  // the first two pages forever.
  //
  // `chips` cannot be the key: screens declare it as an inline array literal,
  // which is a fresh reference each render. The option values are what
  // chosenFor reads, and they are strings.
  // Every chip on the surface, declared and data-driven alike. The owner dial
  // is a dataChip because it names colleagues at runtime, and code that asks
  // which chips there are must see it. Reading `chips` alone once made picking
  // Unassigned send `owner=unassigned:true` instead of `unassigned=true`, and
  // the server answered the whole list.
  const allChips: readonly ListChip[] = [
    ...chips.map((chip) => translateChip(chip, t)),
    ...dataChips,
  ];
  // Keyed on the filters, which are what the list reads. The table treats a
  // new `chosen` identity as the reader narrowing the list and resets to
  // page 1. So this identity must change only when the answer changes.
  //
  // Keying on the chip options made the roster do it. The team dial names the
  // workspace's teams, which arrive on their own query. A chip gained an
  // option seconds after the list rendered and threw the reader from page 2
  // back to page 1 with nothing touched.
  //
  // Keying on chosenFor's result is not enough either, and the saved views are
  // why. A restored view sets `owner_team_id=t-9` before the roster answers.
  // So the filter is already there when the matching option arrives. chosenFor
  // then adds the chip's own key, and the result changes shape. The query and
  // every row stay as they were.
  //
  // The filters are the true signal: the fetch reads them and nothing else.
  // So an identity that follows them resets when the rows change and never
  // otherwise. What a dial offers is presentation, and presentation must not
  // move the reader.
  // Sorted, so re-picking the same value cannot reorder the object and read as
  // a change. setFilter deletes a composite param and re-adds it. That moves
  // its insertion order without moving what it selects.
  const narrowKey = JSON.stringify(
    Object.entries(query.filters).sort(([a], [b]) => stable(a, b)),
  );
  // Rebuilt every render, so a dial shows what is chosen the moment its
  // options arrive. Nothing resets on this (`narrowKey` is the trigger), so a
  // fresh object here costs nothing.
  const chosen = chosenFor(allChips, query.filters);

  // The address moves without this screen unmounting: Back, Forward, a link to
  // the same list narrowed differently. The box has to follow it, or it shows
  // words the rows are not answering. Pressing Back out of a search left the
  // typed word in a box over the unnarrowed list it had just left.
  //
  // Only for a `q` this box did not commit itself. The ref carries what it
  // last put on the wire, so a reader mid-word is never overwritten by their
  // own debounce arriving.
  useEffect(() => {
    if (query.q !== committed.current) {
      committed.current = query.q;
      typed.current = query.q;
      setLocalSearch(query.q);
    }
  }, [query.q]);

  // The reader left with a word still settling. The effect above follows `q`.
  // But the reader may walk away from the list a word was typed against, with
  // a `q` that never moved. Example: Back to a differently sorted view of the
  // same list. The word would then land on a list it was not typed into.
  //
  // The browser's own navigation is the discriminator, with no false hits.
  // `replaceParams` uses `history.replaceState`, which fires neither event, so
  // every dial this surface turns is silent here. Both events are heard: Back
  // moves the address at `popstate` and announces it a task later. A timer
  // firing between them would write the word over the address Back restored.
  useEffect(() => {
    const abandonPendingSearch = () => {
      const live = ownDials(currentParams(), state.paramScope).get("q") ?? "";
      committed.current = live;
      typed.current = live;
      setLocalSearch(live);
    };
    globalThis.addEventListener("popstate", abandonPendingSearch);
    globalThis.addEventListener("hashchange", abandonPendingSearch);
    return () => {
      globalThis.removeEventListener("popstate", abandonPendingSearch);
      globalThis.removeEventListener("hashchange", abandonPendingSearch);
    };
  }, [state.paramScope]);

  useEffect(() => {
    // Only a word the box has not committed is pending. Any other timer would
    // write a word nobody typed over an address that moved while it waited.
    if (!searchable || localSearch === committed.current) {
      return;
    }
    const timer = setTimeout(() => {
      const word = typed.current;
      if (word === committed.current) {
        return;
      }
      committed.current = word;
      // Functional, so a sort, filter or archive toggle set while the timer
      // waited survives instead of being reverted by a stale `query`.
      setQuery((prev) => (prev.q === word ? prev : { ...prev, q: word }));
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [localSearch, setQuery, searchable]);

  // Neither state replaces the surface. The header, the dials and the primary
  // action belong to the screen rather than to the response. So they stay put,
  // and only the body reports what happened.
  const problem = isError ? (
    <>
      <p>{t("common.error")}</p>
      {/* ds:ignore the cause under a headline with its own Retry, a composite */}
      <p className="listquery-cause">{problemMessageOf(error, t)}</p>
      <Button className="listquery-retry" onClick={() => refetch()}>
        {t("common.retry")}
      </Button>
    </>
  ) : undefined;

  // Pressing a tab pins it and restores the list state it holds. The surface
  // rewrites sort and filters; this layer rewrites the ListQuery dials the
  // surface cannot see. Search and the archived toggle are always rewritten.
  // A view describes the whole list, and a leftover would stop the tab
  // matching itself. Page size is the one dial a tab may leave unclaimed, so the reader's
  // own choice survives a tab that carries none.
  const applyViewState = (index: number) => {
    pickView(index);
    const picked = railViews[index];
    if (!picked) {
      return;
    }
    // At once, so a word still settling cannot land on the tab just picked.
    committed.current = picked.q;
    typed.current = picked.q;
    setLocalSearch(picked.q);
    setQuery((prev) => ({
      ...prev,
      q: picked.q,
      includeArchived: picked.includeArchived,
      perPage: picked.perPage ?? prev.perPage,
    }));
  };

  const setFilter = (key: string, value: string) =>
    setQuery((prev) => ({
      ...prev,
      filters: pickedFilters(allChips, prev.filters, key, value),
    }));

  return (
    <ListSurface<Row>
      title={title}
      rows={rows}
      columns={columns}
      rowKey={rowKey}
      onRowClick={rowRoute ? (row) => navigate(rowRoute(row)) : undefined}
      rowHref={rowRoute ? (row) => routeHash(rowRoute(row)) : undefined}
      unit={t(unit)}
      emptyNote={emptyNote}
      action={action}
      caption={caption ? t(caption) : undefined}
      footer={footer}
      tools={tools}
      saveView={saveView}
      search={
        searchable
          ? {
              value: localSearch,
              answered: query.q,
              onChange: (next) => {
                typed.current = next;
                setLocalSearch(next);
              },
            }
          : undefined
      }
      sort={{
        value: query.sort,
        onChange: (next) => setQuery((prev) => ({ ...prev, sort: next })),
      }}
      chips={allChips}
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
      hasMore={state.hasMore}
      hideShortChrome={hideShortChrome}
      onLoadMore={state.loadMore}
      total={state.total}
      // The page size is part of the server query. Changing it re-asks the
      // server, so it lives in the ListQuery the fetchers read their `limit`
      // from, with no second slice on top.
      perPage={query.perPage}
      onPerPage={(next) => setQuery((prev) => ({ ...prev, perPage: next }))}
      // The rendered page is in the address too, so paging through a list and
      // opening a record no longer costs the reader their place. Page one is
      // left out: every list opens there. An address that said so on arrival
      // would put a dial in front of a reader who turned nothing.
      page={listPageOf(params, state.paramScope)}
      onPage={(next) =>
        setParams(withListPage(currentParams(), next, state.paramScope))
      }
      bodyRef={rowsScroller}
      // The unit key names the table for the widths it remembers.
      widthsKey={unit}
      body={body}
      bodyOwnsPaging={bodyOwnsPaging}
      bodyCount={bodyCount}
      selection={selection}
      pending={isPending}
      problem={problem}
    />
  );
}
