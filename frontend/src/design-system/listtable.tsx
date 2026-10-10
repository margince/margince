// The phone layout lays the table's own elements out as cards (listtable.css),
// and a table element laid out as blocks loses its implicit ARIA roles in
// Chrome and Safari. Naming every role explicitly is the fix, so the roles that
// read as redundant in the markup are exactly what keeps the grid announceable
// once the layout changes underneath it.
// biome-ignore-all lint/a11y/noRedundantRoles: display:block drops implicit table roles
// biome-ignore-all lint/a11y/useSemanticElements: the semantic element is already in use

import { ChevronDown, SlidersHorizontal } from "lucide-react";
import { type ReactNode, type RefObject, useRef, useState } from "react";
import {
  formatNumber,
  identifierNumber,
  ordinalNumber,
} from "../format/format";
import { useLocale, useT } from "../i18n";
import { Button, Checkbox, useScrollRegion } from "./atoms";
import {
  CountLine,
  type ListChip,
  ListSurface,
  type ListView,
  Menu,
  nextSortValue,
  type SortControl,
} from "./listsurface";
import {
  type ColumnLayout,
  type ColumnResize,
  floorStyle,
  useColumnWidths,
  useFrozenEdge,
} from "./listtable.layout";
import { PAGE_SIZES, usePaging } from "./listtable.paging";
import {
  applyView,
  clearAll,
  filteredBy,
  narrowedBy,
  type QueryDials,
  sortedColumn,
  sortOptionsOf,
  sortState,
} from "./listtable.query";
import { Select } from "./select";
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

export type ListColumn<Row> = {
  key: string;
  header: string;
  cell: (row: Row) => ReactNode;
  /**
   * The server sort field behind this column. Its presence is what makes the
   * header clickable — a column the API cannot order by stays inert rather
   * than offering a control that would silently do nothing.
   */
  sort?: string;
  /** Right-aligns, and makes the first sort click descending. */
  numeric?: boolean;
  /**
   * Exempt from the Display menu's column list, and the card heading on a
   * phone. The identity column has to stay: it is what makes a row
   * recognisable.
   */
  fixed?: boolean;
  /**
   * This column holds the row's VERBS, not a value. It is then sized by the
   * buttons in it rather than by a share of the table's width — a share the
   * page happens to have room for is not a width two translated labels fit
   * in, and a verb the reader can only half read is a verb they cannot use.
   */
  verbs?: boolean;
  /** Starts unticked in the Display menu: offered rather than drawn. */
  initiallyHidden?: boolean;
};

export type ListSelection<Row> = {
  /** Keys (rowKey) of the selected rows. */
  selected: ReadonlySet<string>;
  onToggle: (row: Row) => void;
  /** The checkbox's accessible name for a row — "Select Anna Weber". */
  label: (row: Row) => string;
  /** Rows the verbs cannot act on carry no checkbox. Default: every row. */
  selectable?: (row: Row) => boolean;
  /** The bulk bar: the count and the verbs. Rendered while anything is selected. */
  bar: ReactNode;
};

/**
 * The selection checkbox in the identity cell. Its click is its own: it must
 * not open the row.
 */
function RowSelect<Row>({
  row,
  rowKey,
  selection,
}: Readonly<{
  row: Row;
  rowKey: (row: Row) => string;
  selection?: ListSelection<Row>;
}>) {
  if (!selection || selection.selectable?.(row) === false) {
    return null;
  }
  return (
    <Checkbox
      className="lt-select"
      label={<span className="sr-only">{selection.label(row)}</span>}
      checked={selection.selected.has(rowKey(row))}
      onChange={() => selection.onToggle(row)}
      onClick={(event) => event.stopPropagation()}
    />
  );
}

/**
 * The identity cell's own row: the selection box and the name side by side,
 * centred on each other.
 *
 * A row of its own because the two are otherwise an inline-grid label followed
 * by a link, and a name long enough to wrap put the checkbox on a line ABOVE the
 * name it selects — which reads as a control belonging to the row above it.
 *
 * The identity cell ONLY. Every other column returns whatever its own renderer
 * returns, blocks included, and a flex `<span>` around those would both invent a
 * layout contract they never asked for and put a `<div>` inside a `<span>`.
 */
function IdentityCell<Row>({
  row,
  rowKey,
  rowHref,
  selection,
  cell,
}: Readonly<{
  row: Row;
  rowKey: (row: Row) => string;
  rowHref?: (row: Row) => string;
  selection?: ListSelection<Row>;
  cell: (row: Row) => ReactNode;
}>) {
  return (
    <span className="lt-identity-row">
      <RowSelect row={row} rowKey={rowKey} selection={selection} />
      {rowHref ? (
        // The identity cell is a real link, so the row can be opened the ways a
        // link can: a new tab, a new window, a bookmark, or the keyboard. Only
        // the default click is stopped from reaching the row's own handler —
        // preventing the anchor instead would navigate the current page while
        // the new tab opens too.
        <a
          className="lt-cellink"
          href={rowHref(row)}
          onClick={(event) => event.stopPropagation()}
        >
          {cell(row)}
        </a>
      ) : (
        cell(row)
      )}
    </span>
  );
}

/**
 * Several pills in one cell, on the row's one line.
 *
 * It does NOT wrap, which is the whole of why it exists: a cell that grows a
 * second line pushes every row below it down, and a page of fifty rows stops
 * being a scannable grid — the reasoning `RowTags` already states for a tag
 * strip. What does not fit ends in an ellipsis, text before badges (why, in
 * listtable.css): the reader's answer to a strip cut short is to widen the
 * column, and every other answer costs the rows their rhythm.
 *
 * A `<span>`, so a column renderer can return it wherever it returns text.
 * Offered rather than applied: the table wraps the identity cell and nothing
 * else, so a column that carries one value keeps whatever layout it returns.
 */
export function CellStrip({ children }: Readonly<{ children: ReactNode }>) {
  return <span className="lt-strip">{children}</span>;
}

/** The bulk bar over the grid, while anything is selected. */
function BulkBar<Row>({
  selection,
}: Readonly<{ selection?: ListSelection<Row> }>) {
  if (!selection || selection.selected.size === 0) {
    return null;
  }
  return <SelectionBar>{selection.bar}</SelectionBar>;
}

/** Page numbers around the current one, which sits in the middle of them. */
const PAGE_WINDOW = 3;

/** Narrow enough to tuck a column away, wide enough to still read a header. */
const MIN_COLUMN_WIDTH = 72;

/** Placeholder rows while the first page loads: enough to read as a list. */
const PLACEHOLDER_ROWS = [0, 1, 2, 3, 4];

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
  /** Omit for a list whose GET has no `q` param; the box is then not rendered. */
  search?: { value: string; onChange: (next: string) => void };
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
  const region = useScrollRegion(scroller, unit);
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

  return (
    <ListSurface
      title={title}
      views={views}
      activeView={activeView}
      onViewChange={(index) => applyView(dials, index)}
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
          {!bodyOwnsPaging && (
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
      {body ?? (
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
            empty={
              <EmptyRow
                unit={unit}
                narrowed={narrowedBy(dials)}
                filtered={filteredBy(dials)}
                onClear={() => clearAll(dials)}
                emptyNote={emptyNote}
              />
            }
          />
        </ListGrid>
      )}
    </ListSurface>
  );
}

/**
 * The presentation dials of the Display menu: which optional columns stand,
 * and how tight the rows are. Local state, unlike the query dials.
 */
function useColumnDisplay<Row>(columns: readonly ListColumn<Row>[]) {
  const [hidden, setHidden] = useState<ReadonlySet<string>>(
    () => new Set(columns.flatMap((c) => (c.initiallyHidden ? [c.key] : []))),
  );
  const [dense, setDense] = useState(false);
  return {
    hidden,
    onToggleColumn: (key: string) =>
      setHidden((prev) => {
        const next = new Set(prev);
        if (next.has(key)) {
          next.delete(key);
        } else {
          next.add(key);
        }
        return next;
      }),
    dense,
    onDense: () => setDense(!dense),
  };
}

/**
 * The scrolling body and its table: the frozen edge, the column widths and the
 * header row. The rows come in as children.
 */
function ListGrid<Row>({
  scrollerRef,
  region,
  frozen,
  head,
  dense,
  layout,
  shown,
  sort,
  children,
}: Readonly<{
  scrollerRef: (node: HTMLDivElement | null) => void;
  region: ReturnType<typeof useScrollRegion>;
  frozen: ReturnType<typeof useFrozenEdge>;
  head: RefObject<HTMLTableElement | null>;
  dense: boolean;
  layout: ColumnLayout & ColumnResize;
  shown: readonly ListColumn<Row>[];
  sort?: SortControl;
  children: ReactNode;
}>) {
  return (
    <div
      className={`lt-scroll${frozen.shifted ? " shifted" : ""}`}
      ref={scrollerRef}
      {...region}
      onScroll={frozen.onScroll}
    >
      {/* One element for the frozen edge's shadow. A shadow per cell starts and
          stops at each cell's box, leaving a seam at every row divider. */}
      <div className="lt-freeze" aria-hidden="true" />
      <table
        ref={head}
        className={`lt-table${dense ? " dense" : ""}${
          layout.resizing ? " is-resizing" : ""
        }`}
        role="table"
        // A custom property rather than `min-width`: the phone layout drops
        // this floor to lay the rows out as cards, and an inline width wins.
        style={floorStyle(layout.floor)}
      >
        {/* Under fixed layout a col wins over the cell below it, so the widths
            live here and a resized column cannot be quietly overruled. */}
        <colgroup>
          {shown.map((column) => (
            <col key={column.key} style={{ width: layout.widthOf(column) }} />
          ))}
          <col style={{ width: layout.slack }} />
        </colgroup>
        <thead role="rowgroup">
          <tr role="row">
            {shown.map((column, index) => (
              <HeaderCell
                key={column.key}
                column={column}
                sort={sort}
                state={sort ? sortState(column, sort.value) : null}
                className={cellClass(column)}
                resizable={index < shown.length - 1}
                onResizeStart={layout.onResizeStart}
                onResize={layout.onResize}
                onResizeEnd={layout.onResizeEnd}
              />
            ))}
            <td className="lt-slack" aria-hidden="true" />
          </tr>
        </thead>
        <tbody role="rowgroup">{children}</tbody>
      </table>
    </div>
  );
}

type RowProps<Row> = Readonly<{
  shown: readonly ListColumn<Row>[];
  rowKey: (row: Row) => string;
  onRowClick?: (row: Row) => void;
  rowHref?: (row: Row) => string;
  selection?: ListSelection<Row>;
}>;

/** The body's rows: placeholders while loading, then the page, the problem or the empty line. */
function GridBody<Row>({
  rows,
  pageRows,
  pending,
  problem,
  empty,
  ...row
}: RowProps<Row> &
  Readonly<{
    rows: readonly Row[];
    pageRows: readonly Row[];
    pending: boolean;
    problem?: ReactNode;
    empty: ReactNode;
  }>) {
  const colSpan = row.shown.length + 1;
  return (
    <>
      {pending &&
        PLACEHOLDER_ROWS.map((placeholder) => (
          <tr key={placeholder} className="lt-loading" role="row">
            {row.shown.map((column) => (
              <td key={column.key} role="cell">
                <span className="skeleton lt-bone" />
              </td>
            ))}
            <td className="lt-slack" aria-hidden="true" />
          </tr>
        ))}
      {!pending &&
        pageRows.map((one) => (
          <DataRow key={row.rowKey(one)} row={one} {...row} />
        ))}
      {!pending && problem && (
        <tr className="lt-empty" role="row">
          <td colSpan={colSpan} role="cell">
            {problem}
          </td>
        </tr>
      )}
      {!pending && !problem && rows.length === 0 && (
        <tr className="lt-empty" role="row">
          <td colSpan={colSpan} role="cell">
            {empty}
          </td>
        </tr>
      )}
    </>
  );
}

/** One record's row. */
function DataRow<Row>({
  row,
  shown,
  rowKey,
  onRowClick,
  rowHref,
  selection,
}: RowProps<Row> & Readonly<{ row: Row }>) {
  return (
    <tr
      role="row"
      className={onRowClick ? "lt-rowlink" : undefined}
      onClick={onRowClick ? () => onRowClick(row) : undefined}
    >
      {shown.map((column) => (
        <td
          key={column.key}
          role="cell"
          className={cellClass(column)}
          // On a phone the rows become cards and the header row is gone, so
          // each value carries its own label. The identity cell is the heading.
          data-label={column.fixed ? undefined : column.header}
        >
          {column.fixed ? (
            <IdentityCell
              row={row}
              rowKey={rowKey}
              rowHref={rowHref}
              selection={selection}
              cell={column.cell}
            />
          ) : (
            column.cell(row)
          )}
        </td>
      ))}
      <td className="lt-slack" aria-hidden="true" />
    </tr>
  );
}

/**
 * The empty line has three states. What narrows the set decides the
 * sentence, and whether it is clearable decides whether Clear is offered.
 * A screen's own scope narrows the set and no button here can clear it.
 */
function EmptyRow({
  unit,
  narrowed,
  filtered,
  onClear,
  emptyNote,
}: Readonly<{
  unit: string;
  narrowed: boolean;
  filtered: boolean;
  onClear: () => void;
  emptyNote?: ReactNode;
}>) {
  const t = useT();
  return (
    <>
      {narrowed ? t("table.noMatches", { unit }) : t("table.none", { unit })}
      {filtered && (
        <>
          {" "}
          <button type="button" className="lt-linkish" onClick={onClear}>
            {t("table.clearFilters")}
          </button>
        </>
      )}
      {/* Under either line, because which emptiness a note explains is the
          caller's to know. A "Mine" view for a reader who owns nothing is a
          narrowed list. */}
      {emptyNote && <p className="lt-empty-note">{emptyNote}</p>}
    </>
  );
}

/**
 * Numeric alignment, and the marker the phone layout uses to promote the
 * identity column to the card's heading. Spelled once so th and td agree.
 */
function cellClass<Row>(column: ListColumn<Row>): string | undefined {
  const names = [
    column.numeric ? "lt-num" : "",
    column.fixed ? "lt-identity" : "",
  ].filter(Boolean);
  return names.length > 0 ? names.join(" ") : undefined;
}

/**
 * A column header. Sortable only when the column names a server sort field,
 * so the arrow never appears on a column the API cannot order by.
 */
function HeaderCell<Row>({
  column,
  sort,
  state,
  className,
  resizable,
  onResizeStart,
  onResize,
  onResizeEnd,
}: Readonly<{
  column: ListColumn<Row>;
  sort?: SortControl;
  state: "asc" | "desc" | null;
  className?: string;
  // False on the trailing column, which carries no divider for the grip to
  // light (`th:nth-last-child(2)` clears it) and no neighbour to take the
  // width from. A grip there drew a line against nothing.
  resizable: boolean;
  onResizeStart: () => void;
  onResize: (key: string, width: number) => void;
  onResizeEnd: () => void;
}>) {
  const t = useT();
  const grip = resizable ? (
    <ResizeGrip
      onStart={onResizeStart}
      onResize={(next) => onResize(column.key, next)}
      onEnd={onResizeEnd}
    />
  ) : null;
  if (!column.sort || !sort) {
    return (
      <th className={className} role="columnheader">
        {column.header}
        {grip}
      </th>
    );
  }
  // The same arithmetic the sort menu presses, so a reader who flips a column
  // here finds that direction there.
  const next = nextSortValue(
    { field: column.sort, numeric: column.numeric },
    state,
  );
  return (
    <th
      className={className}
      role="columnheader"
      aria-sort={
        state === "asc"
          ? "ascending"
          : state === "desc"
            ? "descending"
            : undefined
      }
    >
      <button
        type="button"
        className={`lt-sort${state ? " on" : ""}`}
        aria-label={t("table.sortBy", { column: column.header })}
        onClick={() => sort.onChange(next)}
      >
        {column.header}
        <ChevronDown
          size={12}
          strokeWidth={2}
          aria-hidden="true"
          className={`lt-arrow${state === "asc" ? " up" : ""}`}
        />
      </button>
      {grip}
    </th>
  );
}

/**
 * The handle on a column's trailing edge, dragged with a pointer.
 *
 * It draws nothing at rest. The line a reader sees between two columns is the
 * cell's own `border-right`, already there for every column; a second line
 * inset beside it read as a rendering fault rather than as an affordance. The
 * grip lights THAT edge on hover, and keeps it lit for the length of a drag —
 * `is-dragging` rather than `:hover`, because a pointer dragged past the
 * neighbouring column is no longer over the element it is moving.
 *
 * Deliberately hidden from assistive technology. A labelled control inside a
 * `th` joins that header's accessible name, so every column would announce as
 * "Value, resize the Value column" — the price of a keyboard affordance here is
 * making every header read worse for the contacts who rely on the name most. The
 * Display menu already gives keyboard users control over what a table shows,
 * and a width is presentation rather than content.
 */
function ResizeGrip({
  onStart,
  onResize,
  onEnd,
}: Readonly<{
  onStart: () => void;
  onResize: (width: number) => void;
  onEnd: () => void;
}>) {
  const drag = useRef<{ startX: number; startWidth: number } | null>(null);
  const self = useRef<HTMLSpanElement>(null);
  const [dragging, setDragging] = useState(false);
  const cellWidth = (target: HTMLElement) =>
    target.closest("th")?.getBoundingClientRect().width ?? MIN_COLUMN_WIDTH;

  return (
    <span
      className={`lt-grip${dragging ? " is-dragging" : ""}`}
      ref={self}
      aria-hidden="true"
      // The grip lives inside the header button's cell; without this a drag or
      // a click on it would also sort the column it is resizing.
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => {
        event.stopPropagation();
        event.preventDefault();
        const target: HTMLElement = event.currentTarget;
        target.setPointerCapture(event.pointerId);
        drag.current = {
          startX: event.clientX,
          startWidth: cellWidth(target),
        };
        setDragging(true);
        onStart();
      }}
      onPointerMove={(event) => {
        const from = drag.current;
        if (!from) {
          return;
        }
        onResize(
          Math.max(
            MIN_COLUMN_WIDTH,
            from.startWidth + event.clientX - from.startX,
          ),
        );
      }}
      onPointerUp={(event) => {
        const wasDragging = drag.current !== null;
        drag.current = null;
        setDragging(false);
        event.currentTarget.releasePointerCapture(event.pointerId);
        if (wasDragging) {
          onEnd();
        }
      }}
      // A pointer cancelled mid-drag (a system gesture, a lost capture) never
      // raises pointerup. Without this the table would stay in its dragging
      // dress with nothing moving it — the state the user is looking at has to
      // end wherever the drag did.
      onPointerCancel={() => {
        const wasDragging = drag.current !== null;
        drag.current = null;
        setDragging(false);
        if (wasDragging) {
          onEnd();
        }
      }}
    />
  );
}

/**
 * Everything that decides how the grid is DRAWN rather than what is in it: how
 * tight the rows are, and which optional columns stand. One menu, because the
 * two are one question to a reader — density and a hidden column both answer
 * "show me more of this at once" — and a toolbar that spends a trigger on each
 * spends its right half on dials with nothing grouping them. It goes in the
 * surface's `displayMenu` slot, which hands it the row's own open state: the
 * surface knows nothing of a column or a density, only that one menu on a row
 * is open at a time.
 */
function DisplayMenu<Row>({
  optional,
  hidden,
  onToggleColumn,
  dense,
  onDense,
  open,
  onToggle,
}: Readonly<{
  optional: readonly ListColumn<Row>[];
  hidden: ReadonlySet<string>;
  onToggleColumn: (key: string) => void;
  dense: boolean;
  onDense: () => void;
  open: boolean;
  onToggle: () => void;
}>) {
  const t = useT();
  return (
    <span className="lt-menu-wrap">
      <Button aria-expanded={open} onClick={onToggle}>
        <SlidersHorizontal strokeWidth={1.5} aria-hidden="true" />
        {t("table.display")}
        <ChevronDown className="lt-caret" aria-hidden="true" />
      </Button>
      <Menu open={open} head={t("table.display")} align="right">
        {/* A group is named a rung under the menu's own head: the head says
            which menu this is, a group says which part of it. */}
        <div className="lt-mgroup t-caption">{t("table.density")}</div>
        {/* Density and the column set are both sets the reader builds in one
            visit, so every row is a `Checkbox` and the menu stays open. */}
        <Checkbox
          className="lt-mi"
          checked={dense}
          label={t("table.compact")}
          onChange={onDense}
        />
        {optional.length > 0 && (
          <>
            <div className="lt-mgroup t-caption">{t("table.shownColumns")}</div>
            {optional.map((column) => (
              <Checkbox
                key={column.key}
                className="lt-mi"
                checked={!hidden.has(column.key)}
                label={column.header}
                onChange={() => onToggleColumn(column.key)}
              />
            ))}
          </>
        )}
      </Menu>
    </span>
  );
}

/**
 * A slot in the pager: a page to jump to, a gap where pages were skipped, or
 * the room a gap would take.
 */
export type PagerSlot = number | "gap" | "room";

/**
 * What the pager shows: page one, then the current page between its two
 * neighbours, with gaps marking whatever was skipped between them.
 *
 * Page one is always reachable because it is where a reader who has lost their
 * place goes back to, and walking there one Prev at a time is not going back.
 * The rest is a window rather than every page: a strip that grew a number per
 * read would end up longer than the table it belongs to.
 *
 * A gap marks pages the window skipped and nothing else. Pages the cursor could
 * still fetch are Next's to speak for: marking those with the same dots would
 * give one symbol two meanings, and the reader cannot tell from a gap on the
 * last page whether numbers were hidden or merely never asked for.
 *
 * The slots are a fixed six wide at every position — a gap and the bare ROOM
 * for one are the same width — because a strip that changed width would slide
 * Next out from under the reader between one click and the next.
 */
export function pagerSlots(current: number, lastPage: number): PagerSlot[] {
  const first = Math.min(
    Math.max(1, current - Math.floor(PAGE_WINDOW / 2)),
    Math.max(1, lastPage - PAGE_WINDOW + 1),
  );
  const span = Math.min(PAGE_WINDOW, lastPage - first + 1);
  const window = Array.from({ length: span }, (_, index) => first + index);
  return [
    first > 1 ? 1 : "room",
    first > 2 ? "gap" : "room",
    ...window,
    ...Array.from({ length: PAGE_WINDOW - span }, () => "room" as const),
    window[span - 1] < lastPage ? "gap" : "room",
  ];
}

/**
 * The pager: the pages in hand as numbers, prev/next either side, and the page
 * size on the right. Next stays enabled on the last loaded page while the
 * cursor still has rows to give, which is how the set grows without a total.
 */
function Pager({
  current,
  lastPage,
  hasMore,
  perPage,
  onGoto,
  onPerPage,
}: Readonly<{
  current: number;
  lastPage: number;
  hasMore: boolean;
  perPage: number;
  onGoto: (to: number) => void;
  onPerPage: (next: number) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className={`lt-foot${lastPage === 1 && !hasMore ? " single" : ""}`}>
      {/* A landmark, because this is navigation and a reader who jumps by region
          should find it as one. The numbers carry "Page 3" rather than a bare
          "3": out of the row's context a digit names nothing, and the row's
          context is exactly what a screen reader does not have. */}
      <nav className="lt-pager" aria-label={t("table.pagination")}>
        <button
          type="button"
          disabled={current === 1}
          onClick={() => onGoto(current - 1)}
        >
          {t("table.prev")}
        </button>
        {pagerSlots(current, lastPage).map((slot, index) =>
          typeof slot === "number" ? (
            <button
              type="button"
              key={slot}
              className={slot === current ? "on" : undefined}
              aria-current={slot === current ? "page" : undefined}
              // "page 1.234" would read as a fraction of a page rather than
              // the 1234th of them.
              aria-label={t("table.page", { number: ordinalNumber(slot) })}
              onClick={() => onGoto(slot)}
            >
              {ordinalNumber(slot)}
            </button>
          ) : (
            <span
              // Slots are positional: two of them can hold the same kind of
              // nothing, and neither carries an identity of its own.
              // biome-ignore lint/suspicious/noArrayIndexKey: the position IS the identity
              key={`${slot}-${index}`}
              className="lt-gap"
              aria-hidden="true"
            >
              {slot === "gap" ? "…" : ""}
            </span>
          ),
        )}
        <button
          type="button"
          disabled={current === lastPage && !hasMore}
          onClick={() => onGoto(current + 1)}
        >
          {t("table.next")}
        </button>
      </nav>
      <span className="lt-perpage">
        <Select
          aria-label={t("table.rowsPerPage")}
          value={identifierNumber(perPage)}
          onChange={(next) => onPerPage(Number(next))}
          options={PAGE_SIZES.map((size) => ({
            value: identifierNumber(size),
            label: t("table.perPage", { count: formatNumber(size, locale) }),
          }))}
        />
      </span>
    </div>
  );
}
