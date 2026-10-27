// The phone layout lays the table's own elements out as cards (listtable.css).
// A table element laid out as blocks loses its implicit ARIA roles in Chrome
// and Safari. Naming every role explicitly is the fix. The roles that read as
// redundant in the markup keep the grid announceable once the layout changes.
// biome-ignore-all lint/a11y/noRedundantRoles: display:block drops implicit table roles
// biome-ignore-all lint/a11y/useSemanticElements: the semantic element is already in use

import { type ReactNode, type RefObject, useRef } from "react";
import { useT } from "../i18n";
import { Checkbox } from "./atoms";
import { opensRow } from "./datatable";
import type { ListColumn, ListSelection } from "./listcolumn";
import { useListFocusHandoff } from "./listfocus";
import { HeaderCell } from "./listheader";
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
        // The identity cell is a real link. So the row opens every way a link
        // can (new tab, new window, bookmark, keyboard). Only the
        // default click is stopped from reaching the row's own handler.
        // Preventing the anchor instead would navigate the current page while
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
  layout,
  shown,
  sort,
  children,
}: Readonly<{
  scrollerRef: (node: HTMLDivElement | null) => void;
  region: Readonly<{
    tabIndex: 0 | -1;
    role: "region";
    "aria-label"?: string;
    "aria-labelledby"?: string;
  }>;
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
export function EmptyRow({
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
          <ClearFilters onClear={onClear} label={t("table.clearFilters")} />
        </>
      )}
      {/* Under either line, because which emptiness a note explains is the
          caller's to know. A "Mine" view for a reader who owns nothing is a
          narrowed list. */}
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
