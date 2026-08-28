// The phone layout lays the table's own elements out as cards (listtable.css),
// and a table element laid out as blocks loses its implicit ARIA roles in
// Chrome and Safari. Naming every role explicitly is the fix, so the roles that
// read as redundant in the markup are exactly what keeps the grid announceable
// once the layout changes underneath it.
// biome-ignore-all lint/a11y/noRedundantRoles: display:block drops implicit table roles
// biome-ignore-all lint/a11y/useSemanticElements: the semantic element is already in use

import { Check, ChevronDown, Columns3, Rows3 } from "lucide-react";
import {
  type CSSProperties,
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  formatNumber,
  identifierNumber,
  ordinalNumber,
} from "../format/format";
import { useLocale, useT } from "../i18n";
import { Checkbox, useScrollRegion } from "./atoms";
import {
  CountLine,
  type ListChip,
  ListSurface,
  type ListView,
  Menu,
  nextSortValue,
  type SortControl,
  type SortOption,
  sortDirection,
  useCloseOnEscape,
  useCloseOnOutsideClick,
} from "./listsurface";
import { Select } from "./select";
import "./listtable.css";

export type {
  ListChip,
  ListView,
  SortControl,
  SortOption,
} from "./listsurface";

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
   * Exempt from the column picker, and the card heading on a phone. The
   * identity column has to stay: it is what makes a row recognisable.
   */
  fixed?: boolean;
  /**
   * This column holds the row's VERBS, not a value. It is then sized by the
   * buttons in it rather than by a share of the table's width — a share the
   * page happens to have room for is not a width two translated labels fit
   * in, and a verb the reader can only half read is a verb they cannot use.
   */
  verbs?: boolean;
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
  // No `identity` flag any more: only the identity cell renders this, and a
  // prop that is true at its one call site is a claim the caller can get wrong.
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

/** The bulk bar over the grid, while anything is selected. */
function BulkBar<Row>({
  selection,
}: Readonly<{ selection?: ListSelection<Row> }>) {
  if (!selection || selection.selected.size === 0) {
    return null;
  }
  return (
    <div className="lt-bulkbar" role="region" aria-live="polite">
      {selection.bar}
    </div>
  );
}

/**
 * Page sizes the footer offers. This is the size of a RENDERED page: the caller
 * fetches several of them per read (`listFetchLimit`) and the table slices the
 * rows it holds into pages of this size.
 *
 * The two numbers stay in step because the fetch is always a whole multiple of
 * this one. A buffer sized independently of the page is what once made a list
 * say "1-25 of 50 loaded so far" — two unrelated page sizes on one screen.
 */
const PAGE_SIZES = [25, 50, 100] as const;

/** Page numbers around the current one, which sits in the middle of them. */
const PAGE_WINDOW = 3;

/** Narrow enough to tuck a column away, wide enough to still read a header. */
const MIN_COLUMN_WIDTH = 72;

/**
 * How the table divides its width when the reader has not resized anything.
 *
 * Left to itself a table sizes each column to its content, which reads badly
 * both ways: a name column carrying an avatar, a name and a badge takes half
 * the page, and once that is stopped the whole grid huddles at the left edge
 * with empty space beside it. So the columns take shares of the full width
 * instead, weighted by what they actually hold — a date or an amount is a
 * known short string and never needs more, a name is the one column worth
 * reading in full, and everything else sits between them.
 *
 * The minimums are what a column stops shrinking at: past them the table
 * scrolls sideways rather than crushing the columns.
 *
 * The minimum binds PER COLUMN, not as a sum: a column whose share of the
 * remaining width comes out under its own minimum takes the minimum, and the
 * table grows past its box for it. Held as a sum, the narrow columns stayed
 * narrow — an amount column on a 0.9 share was handed 87px against a 110px
 * floor and cut its own header off, which is the whole defect this comment is
 * standing in front of.
 *
 * A `share` of null means the column does not take part in that division at
 * all: it takes its minimum, in pixels, and the shares divide what is left.
 * A column of BUTTONS is the case — a verb's width is its translated label,
 * which is not a fraction of anything the page decides.
 */
type ColumnSize = Readonly<{ share: number | null; min: number }>;

const COLUMN_SIZES: Readonly<
  Record<"identity" | "numeric" | "standard" | "verbs", ColumnSize>
> = {
  identity: { share: 2.4, min: 200 },
  numeric: { share: 0.9, min: 110 },
  standard: { share: 1.3, min: 130 },
  // Two labelled ghost buttons side by side, plus the cell's own padding, in
  // the longest locale this tree ships: German turns "Edit product" into
  // "Produkt bearbeiten". Sized for that rather than for English, because a
  // width that fits one language and clips another is the same defect this
  // floor exists to prevent — just harder to notice from here.
  verbs: { share: null, min: 320 },
};

/**
 * Whether a fresh reading of the scroller's width should be adopted.
 *
 * The measurement feeds back into itself, and on a platform with classic
 * (non-overlay) scrollbars that feedback can oscillate: the column widths
 * decide whether the body needs a horizontal scrollbar, that bar takes height,
 * the lost height brings in a vertical bar, and the vertical bar takes the very
 * width being measured — which produces the first widths again. React stops
 * such a loop with "Maximum update depth exceeded", and the reader loses the
 * whole list behind an error plate.
 *
 * So a reading that merely returns to the width of a render ago is refused.
 * Both readings are honest views of a box with two stable states; taking the
 * first and holding it leaves the table at most one scrollbar's width narrower
 * than it could be, which is invisible next to losing the list. A genuine
 * resize still lands, because it reports a width that is neither of the two
 * being alternated.
 */
export function widthWorthAdopting(
  next: number,
  current: number,
  beforeThat: number,
): boolean {
  return next !== current && next !== beforeThat;
}

function sizeOf(column: {
  fixed?: boolean;
  numeric?: boolean;
  verbs?: boolean;
}): ColumnSize {
  if (column.fixed) {
    return COLUMN_SIZES.identity;
  }
  if (column.verbs) {
    return COLUMN_SIZES.verbs;
  }
  return column.numeric ? COLUMN_SIZES.numeric : COLUMN_SIZES.standard;
}

/**
 * Column widths outlive the visit: a reader who widened a column to fit their
 * data expects it that way tomorrow, not reset by a reload. Stored per table so
 * two lists never inherit each other's layout, and read defensively — a browser
 * with storage denied still gets a working table, just a forgetful one.
 *
 * The key carries the layout's version: widths written when the columns sized
 * themselves to their content mean something else under shares, and reading
 * them back pins every column at a width nobody chose.
 */
const WIDTHS_PREFIX = "margince.table.widths.v2.";

/**
 * The table's width floor, as the custom property the stylesheet reads.
 *
 * Declared rather than asserted onto `CSSProperties`: React's own type carries
 * the CSS properties it knows, and a cast to it would say this one is among
 * them. The intersection says what is true.
 */
type FloorStyle = CSSProperties & Readonly<{ "--lt-floor": string }>;

function floorStyle(floor: number): FloorStyle {
  return { "--lt-floor": `${floor}px` };
}

function readWidths(key?: string): Record<string, number> {
  if (!key) {
    return {};
  }
  try {
    const raw = localStorage.getItem(WIDTHS_PREFIX + key);
    if (!raw) {
      return {};
    }
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) {
      return {};
    }
    return Object.fromEntries(
      Object.entries(parsed).filter(
        (entry): entry is [string, number] =>
          typeof entry[1] === "number" && Number.isFinite(entry[1]),
      ),
    );
  } catch {
    // A malformed or unreadable entry is not worth failing a table render for;
    // the columns fall back to their content widths.
    return {};
  }
}

function writeWidths(key: string | undefined, widths: Record<string, number>) {
  if (!key) {
    return;
  }
  try {
    localStorage.setItem(WIDTHS_PREFIX + key, JSON.stringify(widths));
  } catch {
    // Storage full or denied: the widths still apply for this visit.
  }
}

/** Placeholder rows while the first page loads: enough to read as a list. */
const PLACEHOLDER_ROWS = [0, 1, 2, 3, 4];

/**
 * Everything narrowing a list, as one value two renders can be compared by.
 *
 * Exported for the tests that pin it: what it must and must not call a change
 * is the whole of why the reset below fires when it does.
 *
 * A caller that declares `narrowKey` has already answered what its narrowing
 * is; the rest are read straight off the dials. `chosen`'s keys are sorted
 * because two objects holding the same filters in a different insertion order
 * are the same narrowing, and a signature that said otherwise would send the
 * reader back to page one for nothing.
 */
export function narrowingSignature(dials: {
  search?: string;
  narrowKey?: string;
  chosen: Readonly<Record<string, string>>;
  perPage: number;
  sort?: string;
  archived?: boolean;
  scopeKey: string;
}): string {
  const narrowedBy =
    dials.narrowKey ??
    Object.keys(dials.chosen)
      // Byte order, not the reader's: this is an identity being compared with
      // itself a render ago, so it has to be the same string in every locale.
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

const EMPTY_FILTERS: Readonly<Record<string, string>> = {};

/** Is this column the one currently sorted, and which way? */
function sortState(
  column: { sort?: string },
  value: string,
): "asc" | "desc" | null {
  return column.sort ? sortDirection(column.sort, value) : null;
}

// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: the alternate body owns one guarded paging branch while the table keeps the shared query controls
export function ListTable<Row>({
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
  note,
  footer,
  hasMore = false,
  onLoadMore,
  perPage: controlledPerPage,
  onPerPage,
  page: controlledPage,
  onPage,
  bodyRef,
  pending = false,
  problem,
  widthsKey,
  tools,
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
  onRowClick?: (row: Row) => void;
  /**
   * Where this row lives, as a URL. Turns the identity cell into a link, so a
   * row can be opened in a new tab or reached by keyboard — a click handler
   * alone can do neither.
   */
  rowHref?: (row: Row) => string;
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
   * dial did passes none — the overlay owner hint goes quiet under a live
   * search for exactly that reason.
   */
  emptyNote?: ReactNode;
  /** Omit for a list whose GET has no `q` param; the box is then not rendered. */
  search?: { value: string; onChange: (next: string) => void };
  /**
   * Omit when the data source refuses to sort. The overlay mirror 422s the
   * dial, so its screens pass nothing and the headers render inert — the
   * table never offers a control the server would reject.
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
  /** Says why the dials are missing, when they are. */
  note?: ReactNode;
  /** An aggregate row under the table, e.g. a count and a total value. */
  footer?: ReactNode;
  /**
   * Whether the server holds rows beyond the ones passed in. Paging is a keyset
   * cursor, so there is no total and no way to jump to an arbitrary page: the
   * pager walks the pages it has, and stepping past the last one fetches the
   * next cursor page rather than pretending a page count it cannot know.
   */
  hasMore?: boolean;
  onLoadMore?: () => void;
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
  /** Appended to the surface's tools slot ahead of the Columns/Compact
   * buttons — a caller's own view-switch or picker, e.g. deals' board/table
   * toggle and pipeline picker. */
  tools?: ReactNode;
}>) {
  const t = useT();
  const [hidden, setHidden] = useState<ReadonlySet<string>>(new Set());
  const [dense, setDense] = useState(false);
  const [columnsOpen, setColumnsOpen] = useState(false);
  const [widths, setWidths] = useState<Readonly<Record<string, number>>>(() =>
    readWidths(widthsKey),
  );
  // A drag reports a width per pointer event, so the two costly steps sit at
  // its edges instead: the other columns are measured once when it starts, and
  // storage is written once when it ends. Doing either per event would read
  // layout back and write to disk a hundred times a second while the reader
  // holds the mouse down. The ref is what the edges read, since a handler
  // mid-drag closes over the state from the render it was created in.
  const live = useRef(widths);
  const applyWidths = (next: Readonly<Record<string, number>>) => {
    live.current = next;
    setWidths(next);
  };
  // Whether a column edge is being dragged right now. The table wears it so
  // the whole grid stops selecting text and keeps the resize cursor under a
  // pointer that has travelled off the grip: a drag that highlighted three
  // rows of names on its way is the reader's answer to "did I grab the right
  // thing", and the answer was no.
  const [resizing, setResizing] = useState(false);
  // The rendered page is the table's own by default, and the caller's when it
  // offers one. It becomes the caller's on a screen that puts the page in the
  // ADDRESS — a reader who paged through a list, opened a record and pressed
  // Back arrived on page one, having lost their place — and the table goes on
  // owning it everywhere else, so no caller is made to hold a number it has
  // nowhere to keep.
  const [ownPage, setOwnPage] = useState(1);
  const page = controlledPage ?? ownPage;
  const setPage = (to: number) => {
    setOwnPage(to);
    onPage?.(to);
  };
  // The page SIZE splits the same way, on the handler rather than on the value:
  // a caller that cannot be told the reader changed it cannot own it. The dial
  // then slices what is already in hand, which is all it can mean without a
  // wire to re-ask. Drawing it disabled instead was the state a preview table
  // shipped in, and a dead control reads as a broken one rather than as a dial
  // this table does not have.
  const [ownPerPage, setOwnPerPage] = useState(
    controlledPerPage ?? PAGE_SIZES[0],
  );
  const perPage = onPerPage ? (controlledPerPage ?? PAGE_SIZES[0]) : ownPerPage;
  const setPerPage = onPerPage ?? setOwnPerPage;
  const scroller = useRef<HTMLDivElement>(null);
  const head = useRef<HTMLTableElement>(null);
  // The frozen column only casts a shadow once columns have actually slid under
  // it. At rest there is nothing behind the edge, and a shadow over open space
  // reads as a seam in the table.
  const [shifted, setShifted] = useState(false);
  // How much room the columns actually have, which only the browser knows: the
  // same table is 654px wide in a settings column and 1342 on a list screen,
  // and the widths below are pixels because a `<col>` cannot express a minimum
  // any other way. Read before paint so the first frame is already right rather
  // than every column starting at its minimum and jumping.
  const [available, setAvailable] = useState(0);
  const [, setResized] = useState(0);
  // The width a render ago, which is what makes the re-measure below safe to
  // run after every render.
  const previous = useRef(0);
  // Re-read after every render, so a body that arrives late — this surface can
  // render a board instead of its own table, and back again — is measured when
  // it does rather than leaving every column pinned at its minimum.
  //
  // Which readings are adopted, and why one is refused: widthWorthAdopting.
  useLayoutEffect(() => {
    if (!scroller.current) {
      return;
    }
    const next = scroller.current.clientWidth;
    if (!widthWorthAdopting(next, available, previous.current)) {
      return;
    }
    previous.current = available;
    setAvailable(next);
  });
  // Re-attached when the scroller itself comes or goes, since an observer left
  // holding a detached node stops following anything. The dep is a trigger
  // rather than a value the effect reads — hence the suppression.
  const drawsOwnTable = body === undefined || body === null;
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only dep
  useEffect(() => {
    const scrolling = scroller.current;
    // Measured once wherever the observer is unavailable (jsdom): the widths
    // are right for the render that just happened, they simply stop following
    // a resize.
    if (!scrolling || typeof ResizeObserver === "undefined") {
      return;
    }
    // The observer asks for a fresh look rather than measuring here, so every
    // width this component adopts passes the oscillation guard above. Bumping a
    // counter is what schedules that look: the layout effect has no dependency
    // list and so runs after any render this causes.
    const observer = new ResizeObserver(() => setResized((n) => n + 1));
    observer.observe(scrolling);
    return () => observer.disconnect();
  }, [drawsOwnTable]);
  // The body is where this surface hides columns, so it is the body that has to
  // be reachable once it does. Named by the noun the count line already uses
  // ("products", "deals") — the one word this component knows the list is OF.
  const region = useScrollRegion(scroller, unit);
  useCloseOnOutsideClick(() => setColumnsOpen(false));
  // The column picker keeps its own open state rather than the surface's, so
  // it needs the same Escape path explicitly — a popover a keyboard cannot
  // dismiss is one a keyboard reader is stuck inside.
  useCloseOnEscape(columnsOpen ? "columns" : null, () => setColumnsOpen(false));

  // One read carries several rendered pages, so the rows the caller holds are a
  // whole multiple of `perPage` and dividing them here lands on page boundaries
  // the reader can reach without waiting for a round trip each.
  const lastPage = Math.max(1, Math.ceil(rows.length / perPage));
  const current = Math.min(page, lastPage);
  const from = (current - 1) * perPage;
  const pageRows = rows.slice(from, from + perPage);

  const shown = columns.filter((column) => !hidden.has(column.key));
  /** What a column takes in pixels outright, before any share is divided. */
  const pinnedWidth = (column: ListColumn<Row>) => {
    const resized = widths[column.key];
    if (resized) {
      return resized;
    }
    const { share, min } = sizeOf(column);
    return share === null ? min : undefined;
  };
  // A column the reader has dragged keeps the width they gave it, and a column
  // of verbs takes its own; the rest divide what is left by their shares, so
  // hiding a column widens the others instead of leaving a gap where it was.
  const shares = shown.reduce((total, column) => {
    const { share } = sizeOf(column);
    return (
      total + (pinnedWidth(column) !== undefined || share === null ? 0 : share)
    );
  }, 0);
  // What the pixel-sized columns have already claimed. The shares divide what
  // is left over, not the whole width: a column fixed in pixels beside shares
  // that added up to the whole width would push the last column off the edge.
  const claimed = shown.reduce(
    (total, column) => total + (pinnedWidth(column) ?? 0),
    0,
  );
  /**
   * Every column's width, in PIXELS, and never below the minimum it declares.
   *
   * Pixels rather than the percentages this once handed the colgroup, for two
   * reasons that are really one: a `<col>` under `table-layout: fixed` honours
   * a bare percentage and nothing else — neither `max(…)` nor a `calc()` mixing
   * `%` with `px`, both of which Chrome discards and replaces with an equal
   * split — so a minimum could not be expressed there, and neither could a
   * share standing beside a column fixed in pixels. The first is why a settings
   * table crushed its amount column to 87px and cut the header off; the second
   * is why dragging one column collapsed every other to the same width.
   *
   * Below the sum of the minimums the row stops shrinking and `.lt-scroll`
   * carries the rest, which is the whole point: a table that ran out of room
   * scrolls, it does not quietly stop showing what is in it.
   */
  const spare = Math.max(0, available - claimed);
  const widthPxOf = (column: ListColumn<Row>) => {
    const pinned = pinnedWidth(column);
    if (pinned !== undefined) {
      return pinned;
    }
    const { share, min } = sizeOf(column);
    // Not rounded: four columns each rounded up is a table one to four pixels
    // wider than the box it sits in, which is a scrollbar over nothing and a
    // tab stop into a region with nothing behind its edge. Fractional pixels
    // sum to exactly what was divided.
    return Math.max(min, (spare * (share ?? 0)) / shares);
  };
  const widthOf = (column: ListColumn<Row>) =>
    shares <= 0 && pinnedWidth(column) === undefined
      ? undefined
      : `${widthPxOf(column)}px`;
  // Where the row stops shrinking, and so where the body starts scrolling.
  const floor = shown.reduce((total, column) => total + widthPxOf(column), 0);
  // What the columns are on screen right now, so a drag can hold the others
  // still at the width they already have. Reads the rendered header rather
  // than recomputing the shares, which is the only thing that knows what a
  // percentage actually came out as.
  const measured = (): Record<string, number> => {
    const cells = head.current?.tHead?.rows[0]?.cells;
    if (!cells) {
      return {};
    }
    return Object.fromEntries(
      shown.map((column, index) => [
        column.key,
        Math.round(cells[index]?.getBoundingClientRect().width ?? 0),
      ]),
    );
  };
  // Where the width goes when the columns do not want all of it — which only
  // happens once the reader has pinned every column narrower than the page.
  // Handing it back to the columns would undo the widths they just set, so a
  // trailing gap takes it. While any column is still on a share this is zero
  // wide, because the shares divide the whole remainder between them.
  const slack = shares === 0 ? undefined : "0px";
  // Which column the server is ordering by, so the header can say so in words
  // rather than leaving a single arrow to carry it.
  const sorted = sort
    ? columns.find((column) => sortState(column, sort.value) !== null)
    : undefined;
  // Every orderable column, not just the shown ones: a reader who hid a column
  // to fit a phone did not give up ordering by it, and the menu is the only
  // route left to that sort once its header is gone.
  const sortOptions: readonly SortOption[] = columns.flatMap((column) =>
    column.sort
      ? [{ field: column.sort, label: column.header, numeric: column.numeric }]
      : [],
  );
  const optional = columns.filter((column) => !column.fixed);
  // What is actually narrowing the set, and so whether an empty result should
  // offer to clear anything. A view is not itself a narrowing: applying one
  // writes its filters into `chosen`, so a view that narrows already reads as
  // narrowed here, and a sort-only view correctly does not. Show-archived is
  // not one either — it WIDENS the set, so an empty list with it on means the
  // records do not exist rather than that a filter is hiding them.
  const filtered =
    Boolean(search?.value) || Object.values(chosen).some(Boolean);
  // Whether ANYTHING is cutting the set down, which is a wider question than
  // whether a CLEARABLE dial is. A screen's own scope — which pipeline's board
  // is being read — narrows the rows and no button here can undo it, so the two
  // answers have to be separate: offering "clear filters" for a scope this
  // table cannot clear would be a control that does nothing, and calling an
  // empty scope "no deals yet" is a claim about the workspace when the truth is
  // that this pipeline has none.
  const narrowed = filtered || Boolean(scopeKey);

  // Narrowing the set changes what page 1 even means, so go back to it rather
  // than stranding the reader on a page that no longer exists. Clamping alone
  // is not enough: filtering 80 rows down to 5 while on page 2 should show the
  // 5, not the last page that still happens to be valid.
  //
  // Re-ordering counts too: page 2 of a list sorted by name holds different
  // records than page 2 of the same list sorted by date, so the reader is
  // looking at rows they never asked for. Same for widening the set with
  // Show archived.
  //
  // Not on ARRIVAL, though: a caller whose page comes from the address would be
  // reset off the page it was addressed with, and a reader following a link to
  // page 6 would watch it become page 1 and the URL rewrite itself.
  //
  // So the reset compares the narrowing to what it was, rather than counting
  // effect runs. Counting runs looks equivalent and is not: an effect runs on
  // arrival, it runs TWICE on arrival under StrictMode, and it runs again for
  // any dep that settles a tick after mount — three different arrivals a
  // run-counter reads as a dial the reader moved. Comparing values has no such
  // gap, because arriving cannot change a value that was read during the same
  // render.
  const narrowing = narrowingSignature({
    search: search?.value,
    narrowKey,
    chosen,
    perPage,
    sort: sort?.value,
    archived: archived?.checked,
    scopeKey,
  });
  // Seeded during the FIRST render, not in the effect, so arrival is never a
  // change no matter how many times the effect runs for it.
  const narrowedBy = useRef(narrowing);
  // `setPage` is re-made every render and the effect must not re-run for that;
  // what it does depends only on the signature.
  // biome-ignore lint/correctness/useExhaustiveDependencies: setPage is stable in effect
  useEffect(() => {
    if (narrowedBy.current === narrowing) {
      return;
    }
    narrowedBy.current = narrowing;
    setPage(1);
    // The PAGE and nothing else. Scrolling the rows back to the top belongs here
    // by the same argument, and must not be done from here: this effect also
    // fires on the way OUT, when the list gets a last render against an address
    // that is no longer its own and every dial it reads is empty. The page
    // survives that — it lives in the address and is derived again on the way
    // back — while an offset written then is memory overwritten, and the reader
    // returns to the top of a list they had scrolled a long way down. The pager
    // scrolls to the top for its own moves, where the trigger is a press and
    // cannot be a teardown.
  }, [narrowing]);

  // The overlay needs two numbers CSS cannot work out for itself: where the
  // frozen column ends, which a reader changes by dragging its grip, and how
  // tall the visible body is. The column set and the row height are the TRIGGER
  // to re-measure, not values the body reads — hence the suppression.
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only deps
  useEffect(() => {
    const body = scroller.current;
    if (!body) {
      return;
    }
    const measure = () => {
      const frozen = body.querySelector("thead .lt-identity");
      const width = frozen ? frozen.getBoundingClientRect().width : 0;
      body.style.setProperty("--lt-freeze", `${Math.round(width)}px`);
      body.style.setProperty("--lt-body", `${body.clientHeight}px`);
    };
    measure();
    // Measured once wherever the observer is unavailable: the numbers are still
    // right for the render that just happened, they simply stop following a
    // resize. A table is worth more than a shadow that tracks perfectly.
    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(body);
    const frozen = body.querySelector("thead .lt-identity");
    if (frozen) {
      observer.observe(frozen);
    }
    return () => observer.disconnect();
  }, [shown.length, dense]);

  const goto = (to: number) => {
    // Stepping past what is loaded asks the server for the next cursor page;
    // the row count grows and the pager grows with it.
    if (to > lastPage && hasMore) {
      onLoadMore?.();
    }
    setPage(Math.max(1, to));
    // The header is sticky and the body scrolls, so without this you land on
    // page 2 already scrolled to its middle.
    if (scroller.current) {
      scroller.current.scrollTop = 0;
    }
  };

  const clearAll = () => {
    search?.onChange("");
    // Whatever is narrowing the list, not only what a chip can name — a filter
    // a view applied without a chip of its own is still one the reader is
    // asking to be rid of.
    for (const key of new Set([
      ...chips.map((chip) => chip.key),
      ...Object.keys(chosen),
    ])) {
      onChipChange?.(key, "");
    }
    onViewChange?.(0);
  };

  const applyView = (index: number) => {
    onViewChange?.(index);
    const view = views[index];
    if (sort) {
      sort.onChange(view?.sort ?? "");
    }
    // Every filter is rewritten, not merged: a view describes the whole filter
    // state, so leaving one from the previous view set would silently narrow
    // the view the user just picked.
    //
    // Keyed on the union rather than on the chips, because a view may narrow by
    // something no chip offers — a lead's minimum score is a number, not a list
    // to pick from — and a loop over the chips alone would drop exactly those,
    // leaving a view that highlights itself and changes nothing.
    const keys = new Set([
      ...chips.map((chip) => chip.key),
      ...Object.keys(chosen),
      ...Object.keys(view?.filters ?? {}),
    ]);
    for (const key of keys) {
      onChipChange?.(key, view?.filters?.[key] ?? "");
    }
  };

  return (
    <ListSurface
      views={views}
      activeView={activeView}
      onViewChange={applyView}
      count={
        !pending &&
        !bodyOwnsPaging && (
          <CountLine
            unit={unit}
            first={from + 1}
            last={from + pageRows.length}
            total={rows.length}
            more={hasMore}
            narrowed={narrowed}
            sortedBy={sorted?.header}
          />
        )
      }
      action={action}
      caption={caption}
      note={note}
      search={search}
      sort={sort}
      sortOptions={sortOptions}
      chips={chips}
      chosen={chosen}
      onChipChange={onChipChange}
      archived={archived}
      tools={
        <>
          {tools}
          {/* Both of TableTools' dials — which columns show, and how tight the
              rows are — describe the GRID. A body that owns its own
              presentation is not drawing one, so offering them there hands the
              reader two controls that visibly do nothing, which is worse than
              their absence: it reads as a broken control rather than as a view
              that has no columns to hide. Withheld on the same condition as the
              count line and the pager, for the same reason. */}
          {!bodyOwnsPaging && (
            <TableTools
              optional={optional}
              hidden={hidden}
              onToggleColumn={(key) =>
                setHidden((prev) => {
                  const next = new Set(prev);
                  if (next.has(key)) {
                    next.delete(key);
                  } else {
                    next.add(key);
                  }
                  return next;
                })
              }
              dense={dense}
              onDense={() => setDense(!dense)}
              open={columnsOpen}
              setOpen={setColumnsOpen}
            />
          )}
        </>
      }
      footer={
        <>
          {footer && <div className="lt-agg">{footer}</div>}
          {!bodyOwnsPaging && (
            <Pager
              current={current}
              lastPage={lastPage}
              hasMore={hasMore}
              perPage={perPage}
              onGoto={goto}
              onPerPage={setPerPage}
            />
          )}
        </>
      }
    >
      <BulkBar selection={selection} />
      {body ?? (
        <div
          className={`lt-scroll${shifted ? " shifted" : ""}`}
          ref={(node) => {
            scroller.current = node;
            if (bodyRef) {
              bodyRef.current = node;
            }
          }}
          {...region}
          onScroll={(event) => {
            const next = event.currentTarget.scrollLeft > 0;
            if (next !== shifted) {
              setShifted(next);
            }
          }}
        >
          {/* One element for the frozen edge's shadow. It cannot hang off the
            cells: a shadow per cell starts and stops at each cell's own box, so
            the column ends up wearing a shadow per row with a seam at every
            divider. This sticks to the left of the scrolling body and spans its
            visible height, which is all that is ever seen of it. */}
          <div className="lt-freeze" aria-hidden="true" />
          <table
            ref={head}
            className={`lt-table${dense ? " dense" : ""}${
              resizing ? " is-resizing" : ""
            }`}
            role="table"
            // Handed over as a custom property rather than as `min-width`
            // itself: an inline width cannot be overridden by a stylesheet, and
            // the phone layout has to drop this floor to lay the rows out as
            // cards that fit the screen.
            style={floorStyle(floor)}
          >
            {/* The widths live here rather than on the header cells: under fixed
              layout a col wins over the cell below it, so one place decides
              and a resized column cannot be quietly overruled. */}
            <colgroup>
              {shown.map((column) => (
                <col key={column.key} style={{ width: widthOf(column) }} />
              ))}
              <col style={{ width: slack }} />
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
                    // Dragging an edge moves that edge. The other columns are
                    // pinned at what they currently measure first, so they stay
                    // where they are and the table itself grows or shrinks —
                    // rather than every column re-dividing the row because one
                    // of them changed.
                    onResizeStart={() => {
                      setResizing(true);
                      applyWidths({ ...measured(), ...live.current });
                    }}
                    onResize={(key, width) =>
                      applyWidths({ ...live.current, [key]: width })
                    }
                    onResizeEnd={() => {
                      setResizing(false);
                      writeWidths(widthsKey, live.current);
                    }}
                  />
                ))}
                <td className="lt-slack" aria-hidden="true" />
              </tr>
            </thead>
            <tbody role="rowgroup">
              {pending &&
                PLACEHOLDER_ROWS.map((placeholder) => (
                  <tr key={placeholder} className="lt-loading" role="row">
                    {shown.map((column) => (
                      <td key={column.key} role="cell">
                        <span className="skeleton lt-bone" />
                      </td>
                    ))}
                    <td className="lt-slack" aria-hidden="true" />
                  </tr>
                ))}
              {!pending &&
                pageRows.map((row) => (
                  <tr
                    key={rowKey(row)}
                    role="row"
                    className={onRowClick ? "lt-rowlink" : undefined}
                    onClick={onRowClick ? () => onRowClick(row) : undefined}
                  >
                    {shown.map((column) => (
                      <td
                        key={column.key}
                        role="cell"
                        className={cellClass(column)}
                        // On a phone the rows become cards and the header row is
                        // gone, so each value carries its own label. The identity
                        // cell is the card's heading and needs none.
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
                ))}
              {!pending && problem && (
                <tr className="lt-empty" role="row">
                  <td colSpan={shown.length + 1} role="cell">
                    {problem}
                  </td>
                </tr>
              )}
              {!pending && !problem && rows.length === 0 && (
                <tr className="lt-empty" role="row">
                  <td colSpan={shown.length + 1} role="cell">
                    {/* Three states, not two. "No rows yet" is a claim about
                        the SET and is false the moment anything is cutting it
                        down — including a narrowing this table cannot clear,
                        which is what a screen's own scope is: switching to a
                        pipeline with no deals said "no deals yet" about a
                        workspace full of them. So what is narrowing decides the
                        sentence, and whether it is CLEARABLE decides whether a
                        verb is offered, because a Clear that cleared nothing
                        would be a control that does nothing. */}
                    {narrowed
                      ? t("table.noMatches", { unit })
                      : t("table.none", { unit })}
                    {filtered && (
                      <>
                        {" "}
                        <button
                          type="button"
                          className="lt-linkish"
                          onClick={clearAll}
                        >
                          {t("table.clearFilters")}
                        </button>
                      </>
                    )}
                    {/* Under EITHER line, because WHICH emptiness a note
                        explains is the caller's to know and not this table's.
                        Drawn only over the unnarrowed one, the case the prop
                        was written for never appeared at all: a "Mine" view
                        for a reader who owns nothing is a NARROWED list. A
                        caller whose note would blame the data source for what
                        the reader's own dial did passes none — the overlay
                        owner hint goes quiet under a live search for exactly
                        that reason. The generic line stays above it either
                        way: "clear filters" undoes every narrowing, and a
                        screen's own way back usually undoes one. */}
                    {emptyNote && (
                      <p
                        className="t-caption"
                        style={{ marginTop: "var(--space-2)" }}
                      >
                        {emptyNote}
                      </p>
                    )}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </ListSurface>
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
 * making every header read worse for the people who rely on the name most. The
 * column picker already gives keyboard users control over what a table shows,
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
 * The table-specific half of the toolbar: the column picker and the density
 * toggle. Passed into ListSurface's `tools` slot — the surface itself has no
 * notion of a column or a row density, only that callers may want a slot
 * there.
 */
function TableTools<Row>({
  optional,
  hidden,
  onToggleColumn,
  dense,
  onDense,
  open,
  setOpen,
}: Readonly<{
  optional: readonly ListColumn<Row>[];
  hidden: ReadonlySet<string>;
  onToggleColumn: (key: string) => void;
  dense: boolean;
  onDense: () => void;
  open: boolean;
  setOpen: (next: boolean) => void;
}>) {
  const t = useT();
  return (
    <>
      {optional.length > 0 && (
        <span className="lt-menu-wrap">
          <button
            type="button"
            className="lt-btn"
            aria-expanded={open}
            onClick={() => setOpen(!open)}
          >
            <Columns3 size={13} strokeWidth={1.5} aria-hidden="true" />
            {t("table.columns")}
          </button>
          <Menu open={open} head={t("table.shownColumns")} align="right">
            {optional.map((column) => (
              <button
                type="button"
                key={column.key}
                className={`lt-mi${hidden.has(column.key) ? "" : " on"}`}
                aria-pressed={!hidden.has(column.key)}
                onClick={() => onToggleColumn(column.key)}
              >
                <span className="lt-cb">
                  <Check size={10} strokeWidth={3} aria-hidden="true" />
                </span>
                {column.header}
              </button>
            ))}
          </Menu>
        </span>
      )}

      <button
        type="button"
        className={`lt-btn${dense ? " on" : ""}`}
        aria-pressed={dense}
        onClick={onDense}
      >
        <Rows3 size={13} strokeWidth={1.5} aria-hidden="true" />
        {t("table.compact")}
      </button>
    </>
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
