// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./factlist.css";

// FactList: label→value pairs a reader scans rather than edits.
//
// Ten sheets already style the same `<dl> > div > dt + dd` shape; a primitive
// nobody can forget to style is the fix, and an eleventh sheet would not be.

/**
 * One row. `term` is what the value is, `value` is the value, and `note` is the
 * qualifier that belongs UNDER the value rather than beside it — how a figure
 * was reached, or that it is partial.
 *
 * Both sides are `ReactNode` because the real rows already carry more than
 * text: an evidence mark, a record link, a status pill, a tabular amount.
 * Formatting stays at the call site, where the locale is.
 */
export type Fact = Readonly<{
  /** Distinguishes rows that repeat the same `term`, which the real data does. */
  key: string;
  term: ReactNode;
  value: ReactNode;
  note?: ReactNode;
  /** Draws the term over the value instead of beside it, for a value that is
   *  a card rather than a line. Only `flat` lists take it. */
  block?: boolean;
}>;

/**
 * A read-only fact table.
 *
 * Rows arrive as an array rather than as children so a caller filters absent
 * facts out before rendering. A `<dt>` with an empty `<dd>` reads as "we know
 * this and it is blank", which is a different claim from not knowing it. Where
 * a row must be shown as unknown, the caller passes the plain words as `value`.
 *
 * `numeric` sets tabular figures on every value, for the case the runtime
 * chip's rows (`.mw-aistat-r dd`) already handle by hand: a column of counts
 * that would otherwise shift as digits change width.
 *
 * `flat` is the list inside a record's details card: a fixed term column so
 * every value starts at one edge, and no rule between rows. A value wraps
 * instead of being cut.
 */
export function FactList({
  facts,
  numeric,
  flat,
  className,
}: Readonly<{
  facts: readonly Fact[];
  numeric?: boolean;
  flat?: boolean;
  className?: string;
}>) {
  return (
    <dl
      className={`factlist${numeric ? " factlist-numeric" : ""}${
        flat ? " factlist-flat" : ""
      }${className ? ` ${className}` : ""}`}
    >
      {facts.map((fact) => (
        <div
          className={
            fact.block ? "factlist-row factlist-row-block" : "factlist-row"
          }
          key={fact.key}
        >
          <dt className="factlist-term">{fact.term}</dt>
          <dd className="factlist-value">
            {fact.value}
            {fact.note !== undefined && (
              <small className="factlist-note t-caption">{fact.note}</small>
            )}
          </dd>
        </div>
      ))}
    </dl>
  );
}
