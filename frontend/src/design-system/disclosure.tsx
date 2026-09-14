// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";

/**
 * A section the reader opens when they want it. The chevron is the only state
 * indicator, so it turns; the styles are in atoms.css beside the other
 * furniture.
 */
export function Disclosure({
  summary,
  action,
  open,
  name,
  className,
  children,
}: Readonly<{
  summary: ReactNode;
  /**
   * One verb belonging to this section, drawn on the summary's line and OUTSIDE
   * the `<summary>` element.
   *
   * That is the whole point of the prop. A `<summary>` is itself the control
   * that opens the section, so a button placed inside it is a control inside a
   * control: axe fails it as `nested-interactive`, and a reader who presses the
   * button also toggles the section under it. Two rail sections had done exactly
   * that, and the verb they nested was the one that opens a form — so pressing
   * "Add employment" collapsed the employments it was about to add to.
   *
   * It stays visible while the section is closed, which is what a section-level
   * verb wants: "Add employment" is a thing to do whether or not the list is on
   * screen.
   */
  action?: ReactNode;
  open?: boolean;
  /**
   * The group this section opens ALONE in. Sections sharing a name are an
   * accordion: the browser closes the others as one opens, with no state in
   * the caller — which is what a column of folds wants when reading one is the
   * point and two open at once would push the list off the pane.
   */
  name?: string;
  className?: string;
  children: ReactNode;
}>) {
  const details = (
    <details
      className={className ? `disclosure ${className}` : "disclosure"}
      open={open}
      name={name}
    >
      <summary className="disclosure-summary">
        <ChevronRight className="disclosure-chevron" aria-hidden="true" />
        <span className="t-label">{summary}</span>
      </summary>
      <div className="disclosure-body">{children}</div>
    </details>
  );
  if (!action) {
    return details;
  }
  return (
    <div className="disclosure-wrap">
      {details}
      <span className="disclosure-action">{action}</span>
    </div>
  );
}
