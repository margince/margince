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
}: Readonly<{
  columns: DataTableColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  onRowClick?: (row: Row) => void;
  /** What the scroll region is called once the table is wider than its box. */
  label: string;
  /** `TableScroll`'s `bleed`: the table spans the `Panel` it stands straight in. */
  bleed?: boolean;
}>) {
  return (
    <TableScroll label={label} bleed={bleed}>
      <table className="table">
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column.key} className={columnClass(column)}>
                {column.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr
              key={rowKey(row)}
              className={onRowClick ? "rowlink" : undefined}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
            >
              {columns.map((column) => (
                <td key={column.key} className={columnClass(column)}>
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
  render: (row: Row) => ReactNode;
  // A column of FIGURES sits against the end of its cell, heading included, so
  // the digits of every row stack into a column the eye runs down; a figure
  // set against the start is a ragged edge of different widths.
  align?: "end";
  // The column that takes the table's spare width — a bar drawn beside the
  // figures it shows. Every other column sizes to its content, so the one that
  // grows is named rather than left to the browser's guess.
  grow?: boolean;
}>;

// Both choices are a cell's own, so the heading and every row wear the same
// class and cannot come apart.
function columnClass<Row>(column: DataTableColumn<Row>): string | undefined {
  const classes = [
    column.align === "end" ? "datatable-end" : "",
    column.grow ? "datatable-grow" : "",
  ].filter(Boolean);
  return classes.length > 0 ? classes.join(" ") : undefined;
}
