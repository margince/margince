// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { usePrefersReducedMotion } from "./motion";

/** Scrolls to the element with this id and focuses it, so the next Tab starts
 * inside it. A button that leads to a section uses this: the router would take
 * a fragment link. */
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
