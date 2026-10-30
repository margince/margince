// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { usePrefersReducedMotion } from "./motion";
import "./jumpbar.css";

/** Scrolls to the element with this id and moves focus there, so the next Tab
 * starts inside it; a verb elsewhere on the page that leads to a section uses
 * this rather than a fragment link the router would take. */
export function useJumpTo(): (id: string) => void {
  const reduced = usePrefersReducedMotion();
  return (id: string) => {
    const section = document.getElementById(id);
    if (!section) {
      return;
    }
    section.scrollIntoView({
      behavior: reduced ? "auto" : "smooth",
      block: "start",
    });
    section.focus({ preventScroll: true });
  };
}

export type JumpTarget = Readonly<{ id: string; label: string }>;

/**
 * The sections of one long page as a row of pills that scroll to each, pinned
 * while the page moves under it. Buttons rather than `#id` links: the app's
 * router owns the hash, and a fragment link would leave the page.
 */
export function JumpBar({
  label,
  targets,
  action,
}: Readonly<{
  label: string;
  targets: readonly JumpTarget[];
  /** The page's one verb, at the row's end edge. */
  action?: ReactNode;
}>) {
  const jump = useJumpTo();
  return (
    <nav className="jumpbar" aria-label={label}>
      <ul className="jumpbar-targets">
        {targets.map((target) => (
          <li key={target.id}>
            <button type="button" onClick={() => jump(target.id)}>
              {target.label}
            </button>
          </li>
        ))}
      </ul>
      {action && <div className="jumpbar-action">{action}</div>}
    </nav>
  );
}
