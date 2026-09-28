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
}: Readonly<{
  columns: { key: string; header: string; render: (row: Row) => ReactNode }[];
  rows: Row[];
  rowKey: (row: Row) => string;
  onRowClick?: (row: Row) => void;
  /** What the scroll region is called once the table is wider than its box. */
  label: string;
}>) {
  return (
    <TableScroll label={label}>
      <table className="table">
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column.key}>{column.header}</th>
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
                <td key={column.key}>{column.render(row)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </TableScroll>
  );
}
