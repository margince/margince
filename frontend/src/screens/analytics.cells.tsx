// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Meter } from "../design-system/readings";
import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";
// BarFigure's layout lives in the screen's sheet; its own module loads it so a
// table drawn outside the screen lays its cells out the same.
import "./analytics.css";

// What every report table's cells are drawn from: the narrowed reads of a
// report row, the count that opens the set it names, and a figure with its bar.
// Its own module so the section tables in their own files read one spelling.

export type ReportRow = components["schemas"]["ReportResult"]["rows"][number];
export type Stage = components["schemas"]["Stage"];

// A figure that names a set, drawn as the way into it. The count stays the
// text — a reader is looking for the number, not for a verb — and the link is
// what the number now is.
export function CountLink({
  count,
  href,
  title,
}: Readonly<{ count: number; href: string; title: string }>) {
  const { locale } = useLocale();
  return (
    <a className="link-button" href={href} title={title}>
      {formatNumber(count, locale)}
    </a>
  );
}

// A figure with its bar, in ONE cell: the bar on the cell's free width and the
// figure at its end, under a heading that names the figure. The bar is drawn
// against the column's own scale and hidden from assistive tech, because the
// figure beside it already says what it draws. A row with no figure draws no
// bar rather than an empty one, which would read as zero.
export function BarFigure({
  value,
  part,
  scale,
  label,
  children,
}: Readonly<{
  value: number | null;
  // A stricter measure the value contains, drawn solid inside it.
  part?: number | null;
  scale: number;
  label: string;
  children: ReactNode;
}>) {
  return (
    <span className="analytics-barfigure">
      <span className="analytics-barfigure-bar" aria-hidden="true">
        {value == null ? null : part == null ? (
          <Meter value={value} max={scale} label={label} flat dense />
        ) : (
          <Meter value={value} part={part} max={scale} label={label} dense />
        )}
      </span>
      {children}
    </span>
  );
}

// The largest figure a column holds, which is the scale its bars share. Rows
// with no figure take no part in it.
export function columnScale(values: readonly (number | null)[]): number {
  return values.reduce<number>(
    (largest, value) => (value == null ? largest : Math.max(largest, value)),
    0,
  );
}

// A report row arrives as `{ [key: string]: unknown }`, so every read narrows.
// These keep the narrowing in one place, and keep the distinction the cells
// depend on: an absent measure is not a zero, and an absent currency is not EUR.
export function rowCurrency(row: ReportRow): string | null {
  // An empty code is not a currency. Left as "" it renders a blank cell where a
  // code belongs, and it groups apart from null while meaning the same thing —
  // which would give the forecast two bands with the same key.
  return typeof row.currency === "string" && row.currency !== ""
    ? row.currency
    : null;
}

export function rowMoney(row: ReportRow, key: string): number | null {
  const value = row[key];
  return value == null ? null : Number(value);
}

export function rowCount(row: ReportRow, key: string): number {
  return Number(row[key] ?? 0);
}
