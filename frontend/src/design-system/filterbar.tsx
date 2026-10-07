// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./filterbar.css";

/**
 * The dials a page's figures are cut by, on one card above them: the filters
 * on the leading edge, the page's own verbs (export, save, record) on the
 * trailing one, and one line under both saying what the figures cover.
 *
 * A card rather than a loose row because the figures below it sit on cards:
 * filters floating on the page ground between them read as part of neither.
 * The caption belongs here too, for the same reason: "results through" is a
 * fact about the cut, and alone on the ground it is one more stray line.
 *
 * A named fieldset, because the dials are only read as one cut together.
 */
export function FilterBar({
  label,
  children,
  actions,
  caption,
}: Readonly<{
  /** What the group is called to a screen reader: the dials' shared noun. */
  label: string;
  /** The dials, in reading order. */
  children: ReactNode;
  /** The page's verbs over the figures, held on the trailing edge. */
  actions?: ReactNode;
  /** One line under the dials about what they selected. */
  caption?: ReactNode;
}>) {
  return (
    <fieldset className="filter-bar" aria-label={label}>
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
