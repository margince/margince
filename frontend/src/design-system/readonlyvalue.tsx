// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";
import "./readonlyvalue.css";
import { useDetailTooltip } from "./tooltip";

/**
 * A value the reader may not change, with the reason why on hover and on focus.
 *
 * A reason takes a tab stop, so a keyboard or touch reader reaches it too,
 * unless `tabStop` is false.
 * It stays the value's description while the tip is shut.
 * A `title` attribute reaches the pointer alone. With no reason the value is
 * plain text and takes no tab stop.
 */
export function ReadOnlyValue({
  reason,
  className,
  describedBy,
  testId,
  tabStop = true,
  children,
}: Readonly<{
  reason?: string;
  /** False for a value in every row of a list, where a stop per row costs a keyboard reader a press each. */
  tabStop?: boolean;
  className?: string;
  /** The id of a reason already on the page, read in place of this one. */
  describedBy?: string;
  testId?: string;
  children: ReactNode;
}>) {
  const { ref, trigger, tip } = useDetailTooltip<HTMLSpanElement>(
    reason ?? "",
    tabStop,
  );
  const reasonId = useId();
  if (!reason) {
    return (
      <span
        className={className}
        aria-describedby={describedBy}
        data-testid={testId}
      >
        {children}
      </span>
    );
  }
  return (
    <>
      <span
        ref={ref}
        className={className ? `readonly-value ${className}` : "readonly-value"}
        data-testid={testId}
        {...trigger}
        aria-describedby={describedBy ?? reasonId}
      >
        {children}
        {tip}
      </span>
      {describedBy === undefined && (
        <span id={reasonId} hidden>
          {reason}
        </span>
      )}
    </>
  );
}
