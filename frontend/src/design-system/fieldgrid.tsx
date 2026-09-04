// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";
import "./fieldgrid.css";

// Whether this grid draws a column for the rows' glyphs. It is the GRID's
// answer rather than each row's, because the column has to exist for every
// row or none: a row that skipped it would start its label where its
// neighbours start their values, and the one left edge is what the grid is
// for. A row in an icon grid with no glyph of its own draws an empty cell.
const IconColumn = createContext(false);

// FieldGrid lays a record's label/value pairs in aligned columns — a fixed
// label rung, the value column taking the rest, and optionally a glyph column
// before them. It is the grid around a value, not the value itself: an
// editable field wraps InlineText or InlineChoice as its children rather than
// this component reimplementing the hover-to-edit affordance those two own.
export function FieldGrid({
  icons = false,
  children,
}: Readonly<{
  // Set when any row carries an `icon`. Opens a column before the labels and
  // widens nothing else, so the label rung keeps its full width — which in a
  // narrow pane is the difference between "Unternehmensgröße" on one line and
  // broken across two.
  icons?: boolean;
  children: ReactNode;
}>) {
  return (
    <IconColumn.Provider value={icons}>
      <div className={icons ? "fieldgrid fieldgrid--icons" : "fieldgrid"}>
        {children}
      </div>
    </IconColumn.Provider>
  );
}

// One label/value pair. `children` is a plain node for a read-only fact,
// InlineText (which draws no visible label of its own — its `label` prop is
// screen-reader- and aria-only), or InlineChoice with `hideLabel` set (which
// otherwise draws its own visible "label: value" and would print the field's
// name twice here). `label` is always required and always drawn in this
// column: the grid's whole point is ONE left edge every value starts at, and
// a row that opts out of the label column to let a child draw its own label
// instead is the row that broke that edge.
export function FieldRow({
  label,
  icon,
  align = "top",
  children,
}: Readonly<{
  label: ReactNode;
  // A glyph naming the KIND of fact, drawn before the label. It is a reading
  // aid for a long column of attributes: a reader looking for the address
  // finds the pin faster than they read nine labels. Decorative by
  // construction — the label beside it is the row's name, so the glyph is
  // hidden from assistive technology and a row without one still reads
  // identically. Pass a lucide element; the grid sizes it.
  icon?: ReactNode;
  // Where the label sits against its value. "top" — the default, and right for
  // every row whose value is text — puts both on the row's first line, so a
  // label or a value that wraps still opens beside its partner. "middle" is
  // for a value that is a BOX rather than a line of text (a lifecycle badge, a
  // chip): taller than the label naming it, and visibly hung too high when the
  // two share a top edge.
  align?: "top" | "middle";
  children: ReactNode;
}>) {
  const hasIconColumn = useContext(IconColumn);
  const modifier = align === "middle" ? " fieldgrid-label--middle" : "";
  const valueModifier = align === "middle" ? " fieldgrid-value--middle" : "";
  return (
    <>
      {hasIconColumn ? (
        <span className={`fieldgrid-icon${modifier}`} aria-hidden="true">
          {icon}
        </span>
      ) : null}
      <span className={`fieldgrid-label${modifier}`}>{label}</span>
      <span className={`fieldgrid-value${valueModifier}`}>{children}</span>
    </>
  );
}
