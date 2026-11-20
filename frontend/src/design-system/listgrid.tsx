// The phone layout lays the table's own elements out as cards (listtable.css).
// A table element laid out as blocks loses its implicit ARIA roles in Chrome
// and Safari. Naming every role explicitly is the fix. The roles that read as
// redundant in the markup keep the grid announceable once the layout changes.
// biome-ignore-all lint/a11y/noRedundantRoles: display:block drops implicit table roles
// biome-ignore-all lint/a11y/useSemanticElements: the semantic element is already in use

import { Fragment, type ReactNode, type RefObject, useRef } from "react";
import { useT } from "../i18n";
import { opensRow } from "../lib/linkclick";
import { useFoldedViewport } from "../lib/viewport";
import { AppLink } from "./applink";
import { Checkbox, type useScrollRegion } from "./atoms";
import type { ListColumn, ListSelection } from "./listcolumn";
import { useListFocusHandoff } from "./listfocus";
import { HeaderCell } from "./listheader";
import { PageSelectBox, type SelectAll } from "./listselection";
import type { SortControl } from "./listsurface";
import { CELL_STRIP_CLASS } from "./listtable.floors";
import {
  type ColumnLayout,
  type ColumnResize,
  floorStyle,
  type useFrozenEdge,
} from "./listtable.layout";
import { sortState } from "./listtable.query";

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
      label={selection.label(row)}
      labelHidden
      checked={selection.selected.has(rowKey(row))}
      onChange={() =>
        selection.onChange([row], !selection.selected.has(rowKey(row)))
      }
      onClick={(event) => event.stopPropagation()}
    />
  );
}

/**
 * The identity cell's own row: the selection box and the name side by side,
 * centred on each other.
 *
 * It is a row of its own because otherwise the two are an inline-grid label
 * followed by a link. A name long enough to wrap put the checkbox on a line
 * above the name it selects. That reads as a control of the row above it.
 *
 * This is for the identity cell only. Every other column returns whatever its
 * own renderer returns, blocks included. A flex `<span>` around those would
 * invent a layout contract they never asked for. It would also put a `<div>`
 * inside a `<span>`.
 */
function IdentityCell<Row>({
  row,
  rowKey,
  rowHref,
  selection,
  children,
}: Readonly<{
  row: Row;
  rowKey: (row: Row) => string;
  rowHref?: (row: Row) => string;
  selection?: ListSelection<Row>;
  children: ReactNode;
}>) {
  return (
    <span className="lt-identity-row">
      <RowSelect row={row} rowKey={rowKey} selection={selection} />
      {rowHref ? (
        // The identity cell is a real link. So the row opens every way a link
        // can (new tab, new window, bookmark, keyboard). Only the
        // default click is stopped from reaching the row's own handler.
        // Preventing the anchor instead would navigate the current page while
        // the new tab opens too.
        <AppLink
          className="lt-cellink"
          href={rowHref(row)}
          onClick={(event) => event.stopPropagation()}
        >
          {children}
        </AppLink>
      ) : (
        children
      )}
    </span>
  );
}

/**
 * Several pills in one cell, on the row's one line.
 *
 * It does not wrap, and that is the whole of why it exists. A cell that grows
 * a second line pushes every row below it down. A page of fifty rows then
 * stops being a scannable grid (`RowTags` states the same reason for a tag
 * strip). What does not fit ends in an ellipsis, text before badges (why, in
 * listtable.css). The reader's answer to a strip cut short is to widen the
 * column, and every other answer costs the rows their rhythm.
 *
 * A `<span>`, so a column renderer can return it wherever it returns text.
 * Offered rather than applied: the table wraps the identity cell and nothing
 * else, so a column that carries one value keeps whatever layout it returns.
 */
export function CellStrip({ children }: Readonly<{ children: ReactNode }>) {
  return <span className={CELL_STRIP_CLASS}>{children}</span>;
}

/** Placeholder rows while the first page loads: enough to read as a list. */
const PLACEHOLDER_ROWS = [0, 1, 2, 3, 4];

/**
 * The scrolling body and its table: the frozen edge, the column widths and the
 * header row. The rows come in as children.
 */
export function ListGrid<Row>({
  scrollerRef,
  region,
  frozen,
  head,
  dense,
  phone,
  layout,
  shown,
  sort,
  all,
  children,
}: Readonly<{
  scrollerRef: (node: HTMLDivElement | null) => void;
  region: ReturnType<typeof useScrollRegion>;
  frozen: ReturnType<typeof useFrozenEdge>;
  head: RefObject<HTMLTableElement | null>;
  dense: boolean;
  /** The rows are phone rows, which `listtable.css` keys on the table. */
  phone: boolean;
  layout: ColumnLayout & ColumnResize;
  shown: readonly ListColumn<Row>[];
  sort?: SortControl;
  /** The selection's page box, drawn in the identity column's header. */
  all?: SelectAll;
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
        }${phone ? " lt-phonerows" : ""}`}
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
                lead={
                  column.fixed && all ? <PageSelectBox all={all} /> : undefined
                }
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
  /** Phone rows: `shown` is the phone row's columns, and a cell says what `folded` gives. */
  phone: boolean;
}>;

/** The body's rows: placeholders while loading, then the page, the problem or the empty line. */
export function GridBody<Row>({
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
            {/* A phone row's placeholder: the name's line and one caption line. */}
            {(row.phone ? row.shown.slice(0, 2) : row.shown).map((column) => (
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
  phone,
}: RowProps<Row> & Readonly<{ row: Row }>) {
  const identity = (column: ListColumn<Row>) => (
    <td key={column.key} role="cell" className={cellClass(column)}>
      <IdentityCell
        row={row}
        rowKey={rowKey}
        rowHref={rowHref}
        selection={selection}
      >
        {phone ? asElement(phoneContent(column, row)) : column.cell(row)}
      </IdentityCell>
    </td>
  );
  return (
    <tr
      role="row"
      className={onRowClick ? "lt-rowlink" : undefined}
      // A control in a cell (an address, a company link) acts on its own,
      // the same rule a DataTable row keeps.
      onClick={
        onRowClick
          ? (event) => {
              if (opensRow(event)) onRowClick(row);
            }
          : undefined
      }
    >
      {phone
        ? phoneCells(row, shown, identity)
        : shown.map((column) =>
            column.fixed ? (
              identity(column)
            ) : (
              <td
                key={column.key}
                role="cell"
                className={cellClass(column)}
                // A card on a phone has no header row, so each value carries its own label.
                data-label={column.header}
              >
                {column.cell(row)}
              </td>
            ),
          )}
      <td className="lt-slack" aria-hidden="true" />
    </tr>
  );
}

/**
 * The empty line has three states. What narrows the set decides the
 * sentence, and whether it is clearable decides whether Clear is offered.
 * A screen's own scope narrows the set and no button here can clear it.
 * A caller's `line` replaces both, for a view that names its own emptiness.
 */
export function EmptyRow({
  unit,
  narrowed,
  filtered,
  onClear,
  line,
  emptyNote,
}: Readonly<{
  unit: string;
  narrowed: boolean;
  filtered: boolean;
  onClear: () => void;
  line?: ReactNode;
  emptyNote?: ReactNode;
}>) {
  const t = useT();
  return (
    <>
      {line ?? (
        <>
          {narrowed
            ? t("table.noMatches", { unit })
            : t("table.none", { unit })}
          {filtered && (
            <>
              {" "}
              <ClearFilters onClear={onClear} label={t("table.clearFilters")} />
            </>
          )}
        </>
      )}
      {/* Under either line: the caller knows which emptiness it explains. */}
      {emptyNote && <p className="lt-empty-note">{emptyNote}</p>}
    </>
  );
}

function ClearFilters({
  onClear,
  label,
}: Readonly<{ onClear: () => void; label: string }>) {
  const button = useRef<HTMLButtonElement>(null);
  useListFocusHandoff(button);
  return (
    <button ref={button} type="button" className="lt-linkish" onClick={onClear}>
      {label}
    </button>
  );
}

type PhonePlace = "title" | "end" | "menu" | "caption";

function phonePlace<Row>(column: ListColumn<Row>): PhonePlace | undefined {
  if (column.fixed) {
    return "title";
  }
  return column.verbs === "menu" ? "menu" : column.phone;
}

/**
 * The columns the grid draws. A list with a `phone` place draws phone rows at
 * the fold, in declared order: the reader's ticks and order are a desktop's.
 */
export function useDrawnColumns<Row>(
  columns: readonly ListColumn<Row>[],
  placed: readonly ListColumn<Row>[],
  hidden: ReadonlySet<string>,
): { phone: boolean; shown: readonly ListColumn<Row>[] } {
  const folded = useFoldedViewport();
  const at = (place: PhonePlace) =>
    columns.filter((column) => phonePlace(column) === place);
  if (!folded || at("end").length + at("caption").length === 0) {
    return {
      phone: false,
      shown: placed.filter((column) => !hidden.has(column.key)),
    };
  }
  return {
    phone: true,
    shown: [
      ...at("title"),
      ...at("end").slice(0, 1),
      ...at("menu"),
      ...at("caption"),
    ],
  };
}

// The phone row's grid places elements; bare text would land in the first free track.
function asElement(content: ReactNode): ReactNode {
  return typeof content === "string" || typeof content === "number" ? (
    <span>{content}</span>
  ) : (
    content
  );
}

function phoneContent<Row>(column: ListColumn<Row>, row: Row): ReactNode {
  const said = column.folded?.(row);
  return said === undefined ? column.cell(row) : said;
}

/** A phone row's cells: line one's, then one caption cell; CSS leaves out a fact that renders nothing. */
function phoneCells<Row>(
  row: Row,
  shown: readonly ListColumn<Row>[],
  identity: (column: ListColumn<Row>) => ReactNode,
): ReactNode[] {
  const cells: ReactNode[] = [];
  const facts: ReactNode[] = [];
  for (const column of shown) {
    const place = phonePlace(column);
    if (place === "title") {
      cells.push(identity(column));
      continue;
    }
    const content = phoneContent(column, row);
    if (place === "caption") {
      facts.push(
        <Fragment key={column.key}>
          {facts.length > 0 && " "}
          <span className="sr-only">{column.header}</span>
          <span className="lt-fact">{content}</span>
        </Fragment>,
      );
    } else {
      cells.push(
        <td
          key={column.key}
          role="cell"
          className={cellClass(column)}
          data-phone={place}
        >
          {content}
        </td>,
      );
    }
  }
  if (facts.length > 0) {
    cells.push(
      <td key="caption" role="cell" data-phone="caption">
        {facts}
      </td>,
    );
  }
  return cells;
}

/**
 * Numeric alignment, and the marker the phone layout uses to promote the
 * identity column to the card's heading. Spelled once so th and td agree.
 */
function cellClass<Row>(column: ListColumn<Row>): string | undefined {
  const names = [
    column.numeric ? "lt-num" : column.verbs === "menu" ? "lt-rowmenu" : "",
    column.fixed ? "lt-identity" : "",
  ].filter(Boolean);
  return names.length > 0 ? names.join(" ") : undefined;
}
