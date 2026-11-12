// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { FlatPanelHeads } from "../../design-system/panel";
import "./analytics.board.css";

/**
 * An Analytics page: the work column beside a rail. The column holds the
 * figures a reader came for; the rail holds what to read and act on next (the
 * written outlook, what needs someone). Below 1100px the rail drops under the
 * column, and on a phone `.analytics-board` lays both out as one list.
 */
export function AnalyticsBoard({
  main,
  rail,
  railLabel,
}: Readonly<{
  main: ReactNode;
  rail?: ReactNode;
  // The rail's name as a landmark: it is a complementary region of the page.
  railLabel: string;
}>) {
  return (
    <div className="analytics-board">
      <div className="analytics-board-main">{main}</div>
      {rail ? (
        <aside className="analytics-board-rail" aria-label={railLabel}>
          <FlatPanelHeads value>{rail}</FlatPanelHeads>
        </aside>
      ) : null}
    </div>
  );
}

/**
 * The work column's sections as one surface: each panel inside loses its own
 * edge and corner, and a hairline parts it from the one above. A column of
 * separate cards read as a row of rival asks; one sheet reads as one review.
 */
export function AnalyticsSheet({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <div className="analytics-sheet">
      <FlatPanelHeads value>{children}</FlatPanelHeads>
    </div>
  );
}
