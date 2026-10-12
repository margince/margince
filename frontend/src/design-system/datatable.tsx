// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown } from "lucide-react";
import {
  Fragment,
  type MouseEvent,
  type ReactNode,
  useId,
  useState,
} from "react";
import { TableScroll } from "./atoms";
import { IconAction } from "./iconaction";
import { type SortableRows, useSortableRows } from "./sortablelist";
import "./atoms.css";
import "./datatable.css";

// The simple table, and its own file because it is a composition rather than an
// atom: a named scroll region, a header row and a row that may be a link. The
// `.table` skin it wears is atoms.css's, shared with the tables drawn by hand;
// datatable.css holds only the class this component alone emits.

export function DataTable<Row>({
  columns: ownColumns,
  rows,
  rowKey,
  rowTestId,
  onRowClick,
  rowOpens,
  detail,
  label,
  bleed,
  fold,
  stickyFirst,
  reorder,
}: Readonly<{
  columns: DataTableColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  rowTestId?: (row: Row) => string;
  /** A press on a control inside the row, or inside a popover it opened, stays that control's. */
  onRowClick?: (row: Row) => void;
  /** Which rows `onRowClick` opens; the rest take no pointer and no press. */
  rowOpens?: (row: Row) => boolean;
  /** Each row opens and closes in place; see `DataTableDetail`. */
  detail?: DataTableDetail<Row>;
  /** The table's name, which a scroll region around it reads from the table. */
  label: string;
  /** `TableScroll`'s `bleed`: the table spans the `Panel` it stands straight in. */
  bleed?: boolean;
  /** Below 36rem of its own width each row folds onto two lines; see `DataTableColumn.fold`. */
  fold?: boolean;
  /** `TableScroll`'s `stickyFirst`: the first column stays put while the rest scrolls. */
  stickyFirst?: boolean;
  /** The reader puts the rows in order by hand; see `DataTableReorder`. */
  reorder?: DataTableReorder<Row>;
}>) {
  const tableId = useId();
  const disclosure = useDisclosure(rows, rowKey, detail);
  const sortable = useSortableRows(rows, rowKey, reorder);
  const columns = [
    ...(reorder ? [gripColumn(reorder.header, sortable.grip)] : []),
    ...ownColumns,
    ...(disclosure ? [disclosure.column] : []),
  ];
  const title = foldTitle(columns);
  // Not native semantics alone: a row laid out as flex loses its table role in Safari.
  const role = (name: string) => (fold ? name : undefined);
  const press = (row: Row) => {
    if (onRowClick) return rowOpens?.(row) === false ? undefined : onRowClick;
    return disclosure?.has(row) ? disclosure.toggleRow : undefined;
  };
  return (
    <TableScroll
      label={{ labelledBy: tableId }}
      bleed={bleed}
      stickyFirst={stickyFirst}
      className={fold ? "table-scroll-fold" : undefined}
    >
      <table
        id={tableId}
        className={sortable.sorting ? "table is-sorting" : "table"}
        role={role("table")}
        aria-label={label}
      >
        <thead role={role("rowgroup")}>
          <tr role={role("row")}>
            {columns.map((column) => (
              <th
                key={column.key}
                className={columnClass(column)}
                role={role("columnheader")}
              >
                {column.headerHidden ? (
                  <span className="sr-only">{column.header}</span>
                ) : (
                  column.header
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody role={role("rowgroup")}>
          {(reorder ? sortable.shown : rows).map((row) => {
            const opens = press(row);
            const key = rowKey(row);
            return (
              <Fragment key={key}>
                <tr
                  ref={reorder ? sortable.rowRef(key) : undefined}
                  className={rowClass(
                    opens !== undefined,
                    disclosure?.isOpen(row) === true,
                    sortable.lifted === key,
                  )}
                  onClick={
                    opens
                      ? (event) => {
                          if (opensRow(event)) opens(row);
                        }
                      : undefined
                  }
                  role={role("row")}
                  data-testid={rowTestId?.(row)}
                >
                  {columns.map((column) => (
                    <td
                      key={column.key}
                      className={columnClass(column)}
                      role={role("cell")}
                      data-fold={fold ? foldPlace(column, title) : undefined}
                    >
                      {cellContent(column, row, fold === true)}
                    </td>
                  ))}
                </tr>
                {disclosure?.rendersRow(row) && (
                  // Mounted while closed so the toggle's aria-controls always resolves.
                  <tr
                    className="datatable-detail"
                    role={role("row")}
                    hidden={!disclosure.isOpen(row)}
                  >
                    <td
                      colSpan={columns.length}
                      id={disclosure.id(row)}
                      role={role("cell")}
                    >
                      {disclosure.isOpen(row) && disclosure.render(row)}
                    </td>
                  </tr>
                )}
              </Fragment>
            );
          })}
        </tbody>
      </table>
      {reorder && sortable.live}
    </TableScroll>
  );
}

/** A row that opens in place: under it with `render`, or in its own cells by `controls`. */
export type DataTableDetail<Row> = Readonly<
  {
    /** The chevron column's heading: read, never drawn. */
    header: string;
    /** The chevron's name. Name the row: a page of rows offers one chevron each. */
    toggleLabel: (row: Row) => string;
    /** A row with nothing to open draws no chevron and ignores a press. */
    has?: (row: Row) => boolean;
  } & (
    | { render: (row: Row) => ReactNode; controls?: never }
    /** The id of what the chevron opens inside the row. */
    | { controls: (row: Row) => string; render?: never }
  ) &
    (
      | { expanded?: never; onToggle?: never }
      /** The open rows' keys, held by the caller. */
      | { expanded: ReadonlySet<string>; onToggle: (key: string) => void }
    )
>;

function useDisclosure<Row>(
  rows: Row[],
  rowKey: (row: Row) => string,
  detail: DataTableDetail<Row> | undefined,
) {
  const base = useId();
  const [own, setOwn] = useState<ReadonlySet<string>>(new Set());
  if (!detail) {
    return undefined;
  }
  const place = new Map(rows.map((row, index) => [rowKey(row), index]));
  const expanded = detail.expanded ?? own;
  const toggleKey =
    detail.onToggle ??
    ((key: string) =>
      setOwn((current) => {
        const next = new Set(current);
        if (!next.delete(key)) next.add(key);
        return next;
      }));
  const has = (row: Row) => detail.has?.(row) ?? true;
  const isOpen = (row: Row) => has(row) && expanded.has(rowKey(row));
  const id = (row: Row) =>
    detail.controls?.(row) ?? `${base}-${place.get(rowKey(row))}`;
  const toggleRow = (row: Row) => toggleKey(rowKey(row));
  const column: DataTableColumn<Row> = {
    key: "datatable-toggle",
    header: detail.header,
    headerHidden: true,
    align: "end",
    fold: "end",
    render: (row) =>
      has(row) && (
        <IconAction
          label={detail.toggleLabel(row)}
          icon={<ChevronDown aria-hidden className="expander-chevron" />}
          disclosure={{ expanded: isOpen(row), controls: id(row) }}
          onClick={() => toggleRow(row)}
        />
      ),
  };
  const render = (row: Row) => detail.render?.(row);
  const rendersRow = (row: Row) => detail.render !== undefined && has(row);
  return { column, has, isOpen, id, toggleRow, render, rendersRow };
}

/** `SortableRows` for a table: a leading grip on each row `can` allows, an empty cell on the rest. */
export type DataTableReorder<Row> = SortableRows<Row> &
  Readonly<{
    /** The grip column's heading: read, never drawn. */
    header: string;
  }>;

const GRIP_COLUMN = "datatable-grip";

function gripColumn<Row>(
  header: string,
  grip: (row: Row) => ReactNode,
): DataTableColumn<Row> {
  return {
    key: GRIP_COLUMN,
    header,
    headerHidden: true,
    fold: "lead",
    render: grip,
  };
}

function rowClass(
  opens: boolean,
  open: boolean,
  lifted: boolean,
): string | undefined {
  const classes = [
    opens ? "rowlink" : "",
    open ? "datatable-open" : "",
    lifted ? "is-lifted" : "",
  ];
  return classes.filter(Boolean).join(" ") || undefined;
}

export type DataTableColumn<Row> = Readonly<{
  key: string;
  header: string;
  /** The heading is read but not drawn: a column of verbs needs no word over it. */
  headerHidden?: boolean;
  render: (row: Row) => ReactNode;
  // A column of FIGURES sits against the end of its cell, heading included, so
  // the digits of every row stack into a column the eye runs down; a figure
  // set against the start is a ragged edge of different widths.
  align?: "end";
  // The column that takes the table's spare width — a bar drawn beside the
  // figures it shows. Every other column sizes to its content, so the one that
  // grows is named rather than left to the browser's guess.
  grow?: boolean;
  // Where the cell goes when a `fold` table folds. "hide" leaves sight but is
  // still read, so it must not hold a control. "lead" keeps its width before
  // the title, empty or not, so every title starts on one edge.
  fold?: "title" | "end" | "hide" | "lead";
  /** The cell once its table folds and the heading is out of sight: a bare figure says its unit. Undefined keeps the cell. */
  folded?: (row: Row) => ReactNode | undefined;
}>;

function cellContent<Row>(
  column: DataTableColumn<Row>,
  row: Row,
  fold: boolean,
): ReactNode {
  const phrase = fold ? column.folded?.(row) : undefined;
  if (phrase === undefined) return column.render(row);
  return (
    <>
      <span className="datatable-unfolded">{column.render(row)}</span>
      <span className="datatable-folded">{phrase}</span>
    </>
  );
}

// React bubbles a portal's click into the row that rendered it, so a target
// outside the row is a popover's. A drag that selects text is not a press.
export function opensRow(event: MouseEvent<HTMLElement>): boolean {
  const target = event.target;
  if (!(target instanceof Element) || !event.currentTarget.contains(target)) {
    return false;
  }
  const control = target.closest(
    "a, button, input, select, textarea, label, summary, [role='button'], [role='switch'], [role='menuitem'], [role='checkbox'], [role='tab']",
  );
  if (control !== null && event.currentTarget.contains(control)) {
    return false;
  }
  return globalThis.getSelection?.()?.isCollapsed !== false;
}

function foldTitle<Row>(columns: DataTableColumn<Row>[]): string | undefined {
  const named = columns.find((column) => column.fold === "title");
  return (named ?? columns.find((column) => column.fold === undefined))?.key;
}

function foldPlace<Row>(
  column: DataTableColumn<Row>,
  title: string | undefined,
): "title" | "end" | "hide" | "lead" | "rest" {
  if (column.key === title) {
    return "title";
  }
  return column.fold === undefined || column.fold === "title"
    ? "rest"
    : column.fold;
}

// Both choices are a cell's own, so the heading and every row wear the same
// class and cannot come apart.
function columnClass<Row>(column: DataTableColumn<Row>): string | undefined {
  const classes = [
    column.align === "end" ? "datatable-end" : "",
    column.grow ? "datatable-grow" : "",
    column.key === GRIP_COLUMN ? "datatable-grip" : "",
  ].filter(Boolean);
  return classes.length > 0 ? classes.join(" ") : undefined;
}
