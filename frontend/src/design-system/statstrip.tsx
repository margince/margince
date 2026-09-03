// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Children, type CSSProperties, type ReactNode } from "react";
import "./statstrip.css";

// The strip's style carries the slot-count custom properties alongside the
// standard properties, so the object literal needs those keys typed rather
// than cast away.
type StripVars = CSSProperties & Record<`--${string}`, string | number>;

// StatStrip is the record page's readings row: ONE comparison of equal slots,
// each a reading pane, with air between them and no plate around them. The
// row is read across as a single comparison, which is what the equal slots
// and the one type scale hold; the panes are what keeps every reading on the
// same translucent ground as every other zone of the page.
//
// It takes StatCards as children and owns only the row: the slot count, the
// gaps between slots, the fold when the row stops being legible, and the one
// type scale every slot in the row shares. A slot that sized itself to its own
// content would stop the row reading as one comparison — some slots carry a
// figure and some carry a sentence.
export function StatStrip({
  children,
  className,
  testId,
  floor,
}: Readonly<{
  children: ReactNode;
  className?: string;
  testId?: string;
  // What qualifies the WHOLE row — a source read to its limit, making every
  // figure above a floor. It belongs to the plate rather than to a slot: the
  // row is read across as one statement, and a caveat attached to one figure
  // invites the reading where the others are exact.
  floor?: ReactNode;
}>) {
  // The column count follows the slots the caller actually drew. A fixed
  // template reserves cells nobody fills, and an empty cell on a plate reads
  // as a reading that failed to load rather than as one this record does not
  // have. `toArray` drops the nulls a conditional slot leaves behind, so the
  // count is what is on screen.
  const slots = Children.toArray(children).length;
  // The fold breakpoints cap at the same count rather than at the sheet's own
  // 3-then-2 ladder: `repeat()` needs an integer, so the cap can only come
  // from here, where the slot count is already known. A two-slot strip folds
  // to two columns at every width instead of inventing a third, empty one.
  const vars: StripVars = {
    "--stat-strip-slots": slots,
    "--stat-strip-slots-3": Math.min(slots, 3),
    "--stat-strip-slots-2": Math.min(slots, 2),
    "--stat-strip-tail-6": tailSpan(slots, 6),
    "--stat-strip-tail-3": tailSpan(slots, 3),
    "--stat-strip-tail-2": tailSpan(slots, 2),
  };
  const row = (
    <section
      className={["stat-strip", className ?? ""].filter(Boolean).join(" ")}
      style={vars}
      data-testid={testId}
    >
      {children}
    </section>
  );
  if (!floor) {
    return row;
  }
  return (
    <div className="stat-strip-wrap">
      {row}
      <p className="t-meta stat-strip-floor">{floor}</p>
    </div>
  );
}

// How many columns the LAST slot occupies, so a folded row never ends in empty
// cells. A strip draws as many slots as the record has readings, and nothing
// makes that count divide the column count: four readings over three columns
// left one slot alone in a row of its own beside two blank cells.
//
// Stretching the tail over what is left of its row is the whole answer, and it
// is arithmetic the component can do because it already knows both numbers —
// unlike "which slot begins a row", which CSS cannot ask (`An+B` takes literal
// integers, so a `var()` there is dropped silently).
//
// The row is still one comparison: every slot on a FULL row is the same width,
// and a wider tail says "this is the rest of the row" rather than "this reading
// is bigger". The alternative — leaving the blanks — says a reading failed to
// load.
function tailSpan(slots: number, cap: number): number {
  const columns = Math.max(1, Math.min(slots, cap));
  return columns - ((slots - 1) % columns);
}
