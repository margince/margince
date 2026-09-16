import type { ReactNode } from "react";
import "./cellstack.css";

/**
 * Two facts in ONE table cell, one under the other.
 *
 * The vertical counterpart to `CellStrip`, and the opposite trade. A strip
 * carries pills that gloss one value, so it refuses a second line and
 * truncates instead — a wrapped strip costs every row below it its place for no
 * reading gained. A stack carries a value and the fact that QUALIFIES it: a
 * failure sentinel under the time it was returned, an attempt count under a
 * held thread's state. Those are two lines to a reader whatever the column is
 * sized to.
 *
 * It exists because the alternative was already in the tree and did not work. A
 * bare `<span>` holding two `<span>`s is inline content with no gap at all, so
 * an operator opening the model-lane table to find out WHY a rung stopped
 * answering read `04/09/2026, 15:29provider_quota` — the timestamp and the
 * sentinel run together into one word.
 *
 * A `<span>`, so a column renderer can return it wherever it returns text.
 */
export function CellStack({ children }: Readonly<{ children: ReactNode }>) {
  return <span className="cell-stack">{children}</span>;
}
