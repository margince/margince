import { type ReactNode, useRef } from "react";
import { AnsweredSearch } from "./answeredsearch";
import { EmptyState, useScrollRegion } from "./atoms";
import {
  CountLine,
  type ListChip,
  ListSurface,
  type ListView,
  type SortControl,
} from "./listsurface";
import {
  useColumnDisplay,
  useColumnWidths,
  useFrozenEdge,
} from "./listtable.layout";
import { fitsOnePage, usePaging } from "./listtable.paging";
import {
  applyView,
  clearAll,
  filteredBy,
  narrowedBy,
  type QueryDials,
  sortedColumn,
  sortOptionsOf,
} from "./listtable.query";
import { Pager } from "./pager";
import "./listtable.css";
import { SelectionBar } from "./selectionbar";

export type {
  ListChip,
  ListView,
  SortControl,
  SortOption,
} from "./listsurface";
export { widthWorthAdopting } from "./listtable.layout";
export { narrowingSignature } from "./listtable.paging";

// The list surface: one component owning the header, the controls, the rows and
// the footer of a record list, so the dials read as belonging to the data they
// act on rather than floating above it.
//
// The query dials are CONTROLLED and server-backed — search, sort and filters
// are reported upward and the caller re-reads the list. Only presentation is
// local state: which columns are shown, how tight the rows are, and which saved
// view is selected. That split is the whole design. A table that quietly sorted
// its own page would be lying about the other pages, and this list is a keyset
// cursor over a set larger than what is loaded.
//
// Generic on purpose: nothing here knows what a CRM record is, which is why
// contacts, companies, leads, deals, products and partners share it.
import type { ListColumn, ListSelection } from "./listcolumn";
import { DisplayMenu } from "./listdisplaymenu";
import { EmptyRow, GridBody, ListGrid } from "./listgrid";

export type { ListColumn, ListSelection } from "./listcolumn";
export { CellStrip } from "./listgrid";

/** The bulk bar over the grid, while anything is selected. */
function BulkBar<Row>({
  selection,
}: Readonly<{ selection?: ListSelection<Row> }>) {
  if (!selection || selection.selected.size === 0) {
    return null;
  }
  return <SelectionBar>{selection.bar}</SelectionBar>;
}

const EMPTY_FILTERS: Readonly<Record<string, string>> = {};

export function ListTable<Row>({
  title,
  bodyCount,
  rows,
  columns,
  rowKey,
  onRowClick,
  rowHref,
  unit,
  emptyNote,
  search,
  sort,
  chips = [],
  chosen = EMPTY_FILTERS,
  narrowKey,
  onChipChange,
  archived,
  views = [],
  activeView = 0,
  onViewChange,
  scopeKey = "",
  action,
  caption,
  footer,
  hasMore = false,
  onLoadMore,
  hideShortChrome = false,
  total: serverTotal,
  perPage: controlledPerPage,
  onPerPage,
  page: controlledPage,
  onPage,
  bodyRef,
  pending = false,
  problem,
  widthsKey,
  tools,
  saveView,
  body,
  bodyOwnsPaging = false,
  selection,
}: Readonly<{
  rows: readonly Row[];
  columns: readonly ListColumn<Row>[];
  rowKey: (row: Row) => string;
  /**
   * Row selection for a bulk action. The checkbox lives INSIDE the identity
   * cell — the one that stays put while the rest scrolls — so a selected row
   * is always recognisable, and the frozen edge, the column widths and the
   * phone cards need no second column. `bar` renders above the grid while
   * anything is selected; the screen puts its verbs there and owns what they
   * do (per-row writes, per-row versions, per-row failures).
   */
  selection?: ListSelection<Row>;
  /**
   * Renders INSTEAD of the grid, keeping the surface's header, search, chips,
   * views and page-size dial exactly where they are.
   *
   * For a second view of the same query — a board over the rows a list already
   * fetched. Swapping the whole surface out for it would take the filter bar
   * with it, leaving the reader looking at a narrowed answer with no way to
   * see or change what narrowed it, which is worse than showing no board.
   */
  body?: ReactNode;
  /** The alternate body carries its own count and continuation control. */
  bodyOwnsPaging?: boolean;
  /**
   * The count sentence for a body that owns its own paging.
   *
   * The SLOT is the same either way — a reader looks for "how much is here" in
   * one place, beside the page's name — and only who can compute it changes.
   * Ignored unless `bodyOwnsPaging`: otherwise the table's own range is true.
   */
  bodyCount?: ReactNode;
  onRowClick?: (row: Row) => void;
  /**
   * Where this row lives, as a URL. Turns the identity cell into a link, so a
   * row can be opened in a new tab or reached by keyboard — a click handler
   * alone can do neither.
   */
  rowHref?: (row: Row) => string;
  /**
   * The page's own name, when this table IS the page. Handed straight to
   * `ListSurface`, which documents what it costs: a screen that passes it
   * heads itself and belongs in `SELF_HEADED_SCREENS`.
   */
  title?: ReactNode;
  /** Plural noun for the count and the empty state — "contacts", "leads". */
  unit: string;
  /**
   * A likelier cause than "there is nothing here", for a caller that knows one.
   *
   * Drawn under the empty state whichever line it carries, narrowed or not:
   * which emptiness a note explains is the CALLER's to know. A "Mine" view for
   * a reader who owns nothing is the case this was written for and is a
   * narrowed list, so a note shown only over the unnarrowed one never appeared.
   * A caller whose note would blame the data source for what the reader's own
   * dial did passes none.
   */
  emptyNote?: ReactNode;
  /**
   * Omit for a list whose GET has no `q` param; the box is then not rendered.
   * `answered` is the search the rows answer, which `value` runs ahead of while
   * the typing settles. A `body` reads it as `AnsweredSearch`.
   */
  search?: {
    value: string;
    answered: string;
    onChange: (next: string) => void;
  };
  /**
   * Omit when the data source refuses to sort: the headers then render inert,
   * so the table never offers a control the server would reject.
   */
  sort?: SortControl;
  chips?: readonly ListChip[];
  chosen?: Readonly<Record<string, string>>;
  /**
   * What the list is narrowed BY, serialized — the reset trigger, separate
   * from `chosen`, which is what the DIALS show.
   *
   * They are two jobs and they cannot share one value. `chosen` must always be
   * current or a dial renders the wrong label; the reset must fire only when
   * the answer changes, or an option arriving late throws the reader off their
   * page. Keying the reset on `chosen` forces one to break the other.
   *
   * Defaults to `chosen` for the callers whose dials are all declared up front
   * and therefore cannot drift apart.
   */
  narrowKey?: string;
  /** Called with "" to clear. */
  onChipChange?: (key: string, value: string) => void;
  archived?: { checked: boolean; onChange: (next: boolean) => void };
  views?: readonly ListView[];
  activeView?: number;
  /**
   * What this list is reading, when the screen narrows it by something that is
   * NOT a chip or a filter. Deals is the case: the pipeline picker is screen
   * state, so switching it changes the whole result set while `chosen` and the
   * filters stay exactly as they were. Page 2 of one pipeline is not page 2 of
   * another, so the reader must land on page 1 — and only the screen knows it.
   */
  scopeKey?: string;
  onViewChange?: (index: number) => void;
  /** The one primary action for this surface, e.g. "New contact". */
  action?: ReactNode;
  /**
   * A standing note about what this list is, when the list needs one. The
   * screen's name is not it: the shell already says which screen you are on,
   * and repeating it here would title the surface twice.
   */
  caption?: ReactNode;
  /** An aggregate row under the table, e.g. a count and a total value. */
  footer?: ReactNode;
  /**
   * Whether the server holds rows beyond the ones passed in. Paging is a keyset
   * cursor, so there is no jumping to an arbitrary page: the pager walks the
   * pages it has, and stepping past the last one fetches the next cursor page.
   */
  hasMore?: boolean;
  onLoadMore?: () => void;
  /**
   * No count, dials or pager while every row fits the smallest page and nothing
   * narrows the list. A dial the reader set can then always be undone.
   */
  hideShortChrome?: boolean;
  /**
   * How many rows match on the server, when it counts them — the difference
   * between "1-25 of 8,372" and "1-25 of 200 loaded so far". Undefined means
   * it does not count, NOT zero, so the line falls back to the rows in hand.
   */
  total?: number;
  /**
   * Rows per RENDERED page. The caller fetches a whole multiple of it, so the
   * table divides the rows it holds on boundaries the fetch already respects.
   * Read only alongside `onPerPage`; without one the table holds the size.
   */
  perPage?: number;
  /**
   * The reader picked a different page size; re-ask the server with it.
   *
   * Omit it, as the page number is omitted, and the table keeps the size
   * itself: with no handler there is no wire to re-ask, so every row is
   * already in hand and slicing them is the whole of what the dial means.
   */
  onPerPage?: (next: number) => void;
  /**
   * Which RENDERED page is on screen, for a caller that keeps it somewhere the
   * table cannot — the address, so a reader who paged through a list and
   * opened a record comes back to the page they left.
   *
   * Omit it and the table holds the number itself, which is what every caller
   * did before this existed. Pass it and the table still tells you when it
   * moves, through `onPage`: the pager, the reset on a new narrowing, and
   * stepping past what is loaded all go through one place.
   */
  page?: number;
  /** The rendered page changed, whether by the pager or by a reset. */
  onPage?: (next: number) => void;
  /**
   * The element the ROWS scroll in, handed back to a caller that has something
   * to do with it — remembering where the reader was, which only the caller
   * knows the history entry for.
   *
   * A full-height list is the one place the page column never moves: the rows
   * take the overflow and the column around them is exactly its own height, so
   * a caller watching the column watches an element that is always at zero.
   * The table keeps owning the scrolling; this is a second reference to the
   * same element, not a second scroller.
   */
  bodyRef?: React.RefObject<HTMLDivElement | null>;
  /**
   * The rows are still loading. The surface keeps its header and controls and
   * puts placeholders in the body: the primary action and the dials belong to
   * the screen, not to the response, and a create button that disappears while
   * a list loads is a button the reader has to wait for.
   */
  pending?: boolean;
  /** Why the rows could not be read, with whatever retry the caller offers. */
  problem?: ReactNode;
  /** Names this table for the column widths it remembers between visits. */
  widthsKey?: string;
  /** After the Display menu: a caller's own view-switch or picker, e.g. deals'
   * board/table toggle. A Save view goes in `saveView`, which stands after. */
  tools?: ReactNode;
  /** Handed to the surface's own last slot; see `ListSurface`. */
  saveView?: ReactNode;
}>) {
  const display = useColumnDisplay(columns);
  const scroller = useRef<HTMLDivElement>(null);
  const head = useRef<HTMLTableElement>(null);
  const shown = columns.filter((column) => !display.hidden.has(column.key));
  // The width observer re-attaches when the scroller comes or goes.
  const drawsOwnTable = body === undefined || body === null;
  const layout = useColumnWidths(
    shown,
    widthsKey,
    { scroller, head },
    drawsOwnTable,
  );
  // The body is where this surface hides columns, so it is the body that has
  // to be reachable. Named by the noun the count line uses.
  const scrolling = useScrollRegion(scroller, unit);
  // Always named, so a list with no title has somewhere to hand focus back to.
  const region = {
    role: "region",
    "aria-label": unit,
    tabIndex: -1,
    ...scrolling,
  } as const;
  const dials: QueryDials = {
    search,
    sort,
    chips,
    chosen,
    onChipChange,
    views,
    onViewChange,
    scopeKey,
  };
  const paging = usePaging(
    {
      rows,
      page: controlledPage,
      onPage,
      perPage: controlledPerPage,
      onPerPage,
      hasMore,
      onLoadMore,
      reachAsked: !bodyOwnsPaging,
    },
    {
      search: search?.value,
      narrowKey,
      chosen,
      sort: sort?.value,
      archived: archived?.checked,
      scopeKey,
    },
    scroller,
  );
  const frozen = useFrozenEdge(scroller, shown.length, display.dense);
  const short =
    hideShortChrome &&
    !pending &&
    !narrowedBy(dials) &&
    archived?.checked !== true &&
    fitsOnePage(rows, serverTotal, hasMore);
  const emptyLine = (
    <EmptyRow
      unit={unit}
      narrowed={narrowedBy(dials)}
      filtered={filteredBy(dials)}
      onClear={() => clearAll(dials)}
      emptyNote={emptyNote}
    />
  );

  return (
    <ListSurface
      title={title}
      views={views}
      activeView={activeView}
      onViewChange={(index) => applyView(dials, index)}
      bare={short}
      count={
        bodyOwnsPaging
          ? bodyCount
          : !pending && (
              <CountLine
                unit={unit}
                first={paging.from + 1}
                last={paging.from + paging.pageRows.length}
                total={serverTotal ?? rows.length}
                // "Loaded so far" is the caveat for a number the client
                // counted itself; an exact total needs none.
                more={hasMore && serverTotal === undefined}
                sortedBy={sortedColumn(columns, sort)?.header}
              />
            )
      }
      action={action}
      caption={caption}
      search={search}
      sort={sort}
      sortOptions={sortOptionsOf(columns)}
      chips={chips}
      chosen={chosen}
      onChipChange={onChipChange}
      archived={archived}
      tools={
        <>
          {/* Both of the Display menu's dials describe the grid. A body that
              draws its own presentation would show controls that do nothing,
              so they are withheld with the count line and the pager. */}
          {tools}
        </>
      }
      displayMenu={
        bodyOwnsPaging
          ? undefined
          : (menu) => (
              <DisplayMenu
                optional={columns.filter((column) => !column.fixed)}
                {...display}
                {...menu}
              />
            )
      }
      saveView={saveView}
      footer={
        <>
          {footer && <div className="lt-agg">{footer}</div>}
          {!bodyOwnsPaging && !short && (
            <Pager
              current={paging.current}
              lastPage={paging.lastPage}
              hasMore={hasMore}
              perPage={paging.perPage}
              onGoto={paging.goto}
              onPerPage={paging.setPerPage}
            />
          )}
        </>
      }
    >
      <BulkBar selection={selection} />
      {short && !problem && rows.length === 0 ? (
        <div className="lt-emptyline">
          <EmptyState>{emptyLine}</EmptyState>
        </div>
      ) : body != null ? (
        <AnsweredSearch.Provider value={search?.answered}>
          {body}
        </AnsweredSearch.Provider>
      ) : (
        <ListGrid
          scrollerRef={(node) => {
            scroller.current = node;
            if (bodyRef) {
              bodyRef.current = node;
            }
          }}
          region={region}
          frozen={frozen}
          head={head}
          dense={display.dense}
          layout={layout}
          shown={shown}
          sort={sort}
        >
          <GridBody
            rows={rows}
            pageRows={paging.pageRows}
            shown={shown}
            rowKey={rowKey}
            onRowClick={onRowClick}
            rowHref={rowHref}
            selection={selection}
            pending={pending}
            problem={problem}
            empty={emptyLine}
          />
        </ListGrid>
      )}
    </ListSurface>
  );
}
