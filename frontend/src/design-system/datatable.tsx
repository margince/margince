// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { TableScroll } from "./atoms";
import "./atoms.css";
import "./datatable.css";

// The simple table, and its own file because it is a composition rather than an
// atom: a named scroll region, a header row and a row that may be a link. The
// `.table` skin it wears is atoms.css's, shared with the tables drawn by hand;
// datatable.css holds only the class this component alone emits.

export function DataTable<Row>({
  columns,
  rows,
  rowKey,
  onRowClick,
  label,
  bleed,
  fold,
  stickyFirst,
}: Readonly<{
  columns: DataTableColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  onRowClick?: (row: Row) => void;
  /** What the scroll region is called once the table is wider than its box. */
  label: string;
  /** `TableScroll`'s `bleed`: the table spans the `Panel` it stands straight in. */
  bleed?: boolean;
  /** Below 36rem of its own width each row folds onto two lines; see `DataTableColumn.fold`. */
  fold?: boolean;
  /** `TableScroll`'s `stickyFirst`: the first column stays put while the rest scrolls. */
  stickyFirst?: boolean;
}>) {
  const title = foldTitle(columns);
  // Not native semantics alone: a row laid out as flex loses its table role in Safari.
  const role = (name: string) => (fold ? name : undefined);
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
            <tr
              key={rowKey(row)}
              className={onRowClick ? "rowlink" : undefined}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
              role={role("row")}
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
          ))}
        </tbody>
      </table>
    </TableScroll>
  );
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
