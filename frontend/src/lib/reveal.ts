// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The other kind of door.
//
// A reading's way out either NAVIGATES — `navigate` in ./router, a different
// address — or REVEALS: the rows it was read from are already on the page, and
// what the reader needs is to be taken to them. Beside the router because the
// two are one decision made twice, and a reader pressing "Open" cannot tell
// which of them answered.

/**
 * A handler that scrolls the element with this id into view.
 *
 * `scrollIntoView` and not a fragment href: this app routes on the hash, so
 * `#deal-offers` would be read as an ADDRESS and take the reader off the
 * record.
 *
 * The id is ONE constant, exported by the strip that draws the door and given
 * to the element by the screen that owns the layout, so a typo cannot split the
 * two. The only miss left is a section not on the page — and a door for such a
 * section is not drawn: a withheld deal strip, say, and every other
 * door stands in the same tree as its section. The `?.` on the lookup answers
 * the type, `HTMLElement | null`, not a case this app has.
 */
export function reveal(anchor: string): () => void {
  return () => {
    // jsdom has no scrollIntoView; the browser always does.
    document
      .getElementById(anchor)
      ?.scrollIntoView?.({ behavior: "smooth", block: "start" });
  };
}

/**
 * Reveals an anchor that may not be on the page yet: a press that also
 * switches to the tab rendering it finds nothing on the first frame. Looked
 * for once per frame for a moment, then given up rather than kept forever.
 */
export function revealOnceMounted(anchor: string, frames = 30): void {
  const look = (left: number) => {
    const target = document.getElementById(anchor);
    if (target) {
      target.scrollIntoView?.({ behavior: scrollBehavior(), block: "start" });
      return;
    }
    if (left > 0) {
      requestAnimationFrame(() => look(left - 1));
    }
  };
  look(frames);
}

/**
 * Takes the reader back to the top of the page pane, smoothly. A door in one
 * tab's body that opens another tab uses it, so the reader does not land
 * part-way down the new one.
 *
 * `.scroll` is the shell's one scrolling pane (app/shell.css); the window
 * itself never scrolls. Motion follows the reader's own setting: a reduced-
 * motion preference makes the jump instant rather than skipping it.
 */
export function scrollPageToTop(): void {
  const pane = document.querySelector(".scroll");
  pane?.scrollTo?.({ top: 0, behavior: scrollBehavior() });
}

export function placeAfterTabChange(
  strip: HTMLElement,
): { pane: HTMLElement; top: number } | null {
  const pane = strip.closest<HTMLElement>(".scroll");
  if (!pane) return null;
  // The pane runs up behind the top bar by the bar's height (app/shell.css).
  const underBar = Math.max(
    0,
    -Number.parseFloat(getComputedStyle(pane).marginTop) || 0,
  );
  const visibleTop = pane.getBoundingClientRect().top + underBar;
  const above = visibleTop - strip.getBoundingClientRect().top;
  return { pane, top: pane.scrollTop - Math.max(0, above) };
}

// A loading body is too short for `top`, and the clamp outlasts its growth.
// Not useScrollMemory: it keys on history entries, and a deal's tab adds none.
export function holdScrollTop(pane: HTMLElement, top: number): () => void {
  const start = pane.scrollTop;
  const clamped = () =>
    pane.scrollTop < top &&
    pane.scrollTop >= pane.scrollHeight - pane.clientHeight - 1;
  const gliding = () => pane.scrollTop < start && pane.scrollTop > top;
  const gestures = ["wheel", "touchstart", "keydown"] as const;
  const growth = new MutationObserver(() => {
    if (clamped()) pane.scrollTo?.({ top, behavior: "auto" });
  });
  const moved = () => {
    if (Math.abs(pane.scrollTop - top) >= 1 && !clamped() && !gliding()) {
      release();
    }
  };
  const release = () => {
    growth.disconnect();
    pane.removeEventListener("scroll", moved);
    for (const gesture of gestures) pane.removeEventListener(gesture, release);
  };
  growth.observe(pane, { childList: true, subtree: true });
  pane.addEventListener("scroll", moved, { passive: true });
  for (const gesture of gestures) {
    pane.addEventListener(gesture, release, { passive: true });
  }
  if (Math.abs(start - top) >= 1) {
    // Upward is the strip coming back into sight, so it may glide.
    const behavior = top < start ? scrollBehavior() : "auto";
    pane.scrollTo?.({ top, behavior });
  }
  return release;
}

// Motion follows the reader's own setting: a reduced-motion preference makes
// every reveal on this page an instant jump rather than a glide.
function scrollBehavior(): ScrollBehavior {
  const reduced = globalThis.matchMedia?.(
    "(prefers-reduced-motion: reduce)",
  ).matches;
  return reduced ? "auto" : "smooth";
}
