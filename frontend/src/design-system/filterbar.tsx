// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./filterbar.css";

/**
 * The dials a page's figures are cut by, on one card above them. The filters
 * sit on the leading edge and the page's own verbs (export, save, record) on
 * the trailing one. One line under both says what the figures cover.
 *
 * A card rather than a loose row, because the figures below it sit on cards.
 * Filters floating on the page ground between them read as part of neither.
 * The caption belongs here for the same reason. "Results through" is a fact
 * about the cut, and alone on the ground it is one more stray line.
 *
 * A named fieldset, because the dials are only read as one cut together.
 */
export function FilterBar({
  label,
  children,
  actions,
  caption,
  quiet = false,
}: Readonly<{
  /** What the group is called to a screen reader: the dials' shared noun. */
  label: string;
  /** The dials, in reading order. */
  children: ReactNode;
  /** The page's verbs over the figures, held on the trailing edge. */
  actions?: ReactNode;
  /** One line under the dials about what they selected. */
  caption?: ReactNode;
  /** No card: the dials sit on the page ground, for a view whose figures should carry more weight than its controls. */
  quiet?: boolean;
}>) {
  return (
    <fieldset
      className="filter-bar"
      aria-label={label}
      data-quiet={quiet || undefined}
    >
      <div className="filter-bar-row">
        <div className="filter-bar-controls">{children}</div>
        {actions != null && <div className="filter-bar-actions">{actions}</div>}
      </div>
      {caption != null && (
        <p className="filter-bar-caption t-caption" role="status">
          {caption}
        </p>
      )}
    </fieldset>
  );
}
