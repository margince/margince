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
  detail,
  label,
  bleed,
  fold,
  stickyFirst,
}: Readonly<{
  columns: DataTableColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  rowTestId?: (row: Row) => string;
  /** A press on a control inside the row, or inside a popover it opened, stays that control's. */
  onRowClick?: (row: Row) => void;
  /** Each row opens and closes in place; see `DataTableDetail`. */
  detail?: DataTableDetail<Row>;
  /** What the scroll region is called once the table is wider than its box. */
  label: string;
  /** `TableScroll`'s `bleed`: the table spans the `Panel` it stands straight in. */
  bleed?: boolean;
  /** Below 36rem of its own width each row folds onto two lines; see `DataTableColumn.fold`. */
  fold?: boolean;
  /** `TableScroll`'s `stickyFirst`: the first column stays put while the rest scrolls. */
  stickyFirst?: boolean;
}>) {
  const disclosure = useDisclosure(rows, rowKey, detail);
  const columns = disclosure ? [...ownColumns, disclosure.column] : ownColumns;
  const title = foldTitle(columns);
  // Not native semantics alone: a row laid out as flex loses its table role in Safari.
  const role = (name: string) => (fold ? name : undefined);
  const press = onRowClick ?? disclosure?.toggleRow;
  return (
    <TableScroll
      label={label}
      bleed={bleed}
      stickyFirst={stickyFirst}
      className={fold ? "table-scroll-fold" : undefined}
    >
      <table className="table" role={role("table")}>
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
          {rows.map((row) => (
            <Fragment key={rowKey(row)}>
              <tr
                className={rowClass(
                  onRowClick !== undefined || disclosure?.has(row) === true,
                  disclosure?.isOpen(row) === true,
                )}
                onClick={
                  press
                    ? (event) => {
                        if (opensRow(event)) press(row);
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
                    {column.render(row)}
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
                    {disclosure.isOpen(row) && detail?.render?.(row)}
                  </td>
                </tr>
              )}
            </Fragment>
          ))}
        </tbody>
      </table>
    </TableScroll>
  );
}

/**
 * A row that opens in place. With `render`, what it opens is a full-width row
 * under it; without, the caller's own cells read the open keys and `controls`
 * names the element they open. A chevron at the row's end toggles it, and so
 * does a press anywhere else on the row.
 */
export type DataTableDetail<Row> = Readonly<{
  /** The toggle column's heading: read, never drawn. */
  header: string;
  /** The toggle's name. Name the row: a page of rows offers one toggle each. */
  toggleLabel: (row: Row) => string;
  render?: (row: Row) => ReactNode;
  /** A row with nothing to open draws no toggle and ignores a press. */
  has?: (row: Row) => boolean;
  /** The open rows' keys, held by the caller together with `onToggle`. */
  expanded?: ReadonlySet<string>;
  onToggle?: (key: string) => void;
  /** Without `render`: the id of what the toggle opens inside the row. */
  controls?: (row: Row) => string;
}>;

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
    detail.controls?.(row) ?? `${base}-${rows.indexOf(row)}`;
  const toggleRow = (row: Row) => {
    if (has(row)) toggleKey(rowKey(row));
  };
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
  const rendersRow = (row: Row) => detail.render !== undefined && has(row);
  return { column, has, isOpen, id, toggleRow, rendersRow };
}

function rowClass(opens: boolean, open: boolean): string | undefined {
  const classes = [opens ? "rowlink" : "", open ? "datatable-open" : ""];
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
  // still read, so it must not hold a control.
  fold?: "title" | "end" | "hide";
}>;

// React bubbles a click out of a portal into the row that rendered it, so a
// target outside the row is a popover's, never the row's. A drag that selected
// text to copy is not a press either.
function opensRow(event: MouseEvent<HTMLTableRowElement>): boolean {
  const target = event.target;
  if (!(target instanceof Element) || !event.currentTarget.contains(target)) {
    return false;
  }
  const control = target.closest(
    "a, button, input, select, textarea, label, [role='button']",
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
): "title" | "end" | "hide" | "rest" {
  if (column.key === title) {
    return "title";
  }
  return column.fold === "end" || column.fold === "hide" ? column.fold : "rest";
}

// Both choices are a cell's own, so the heading and every row wear the same
// class and cannot come apart.
function columnClass<Row>(column: DataTableColumn<Row>): string | undefined {
  const classes = [
    column.align === "end" ? "datatable-end" : "",
    column.grow ? "datatable-grow" : "",
  ].filter(Boolean);
  return classes.length > 0 ? classes.join(" ") : undefined;
}
