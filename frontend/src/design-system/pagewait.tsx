// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { usePrefersReducedMotion } from "./motion";
import { useWaitedWhile } from "./waited";
import "./pagewait.css";

/** A shorter wait shows nothing, so a quick move does not blink. */
export const PAGE_WAIT_DELAY_MS = 200;

/** Whether the next page has kept the reader waiting long enough to say so. */
export function usePageWait(pending: boolean): boolean {
  return useWaitedWhile(pending, PAGE_WAIT_DELAY_MS);
}

/**
 * The line under the top bar while the next page loads, and the status that
 * names it. The region stays mounted, so a screen reader hears it fill.
 */
export function PageWait({
  waiting,
  label,
  reducedMotion,
}: Readonly<{
  /** From `usePageWait`, so the line and the column's `aria-busy` agree. */
  waiting: boolean;
  /** What is loading, such as "Loading Account…"; mounted only while waiting. */
  label: ReactNode;
  /** Overrides the media query, so a story can show the still line. */
  reducedMotion?: boolean;
}>) {
  const prefersReduced = usePrefersReducedMotion();
  const still = reducedMotion ?? prefersReduced;
  return (
    <div className="pagewait" role="status">
      {waiting && (
        <>
          <span className="pagewait-line" data-motion={still ? "none" : "in"} />
          <span className="sr-only">{label}</span>
        </>
      )}
    </div>
  );
}
