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

/**
 * How far below the top bar `strip` stands in its page pane, and that pane.
 * A strip above the screen counts as standing right under the bar.
 */
export function stripPlace(
  strip: HTMLElement,
): { pane: HTMLElement; below: number } | null {
  const pane = strip.closest<HTMLElement>(".scroll");
  if (!pane) return null;
  const below = strip.getBoundingClientRect().top - visibleTop(pane);
  return { pane, below: Math.max(0, below) };
}

/** The pane offset that stands `strip` `below` px under the top bar. */
export function offsetShowingStrip(
  pane: HTMLElement,
  strip: HTMLElement,
  below: number,
): number {
  const shift = strip.getBoundingClientRect().top - visibleTop(pane) - below;
  return Math.max(0, pane.scrollTop + shift);
}

function visibleTop(pane: HTMLElement): number {
  // The pane runs up behind the top bar by the bar's height (app/shell.css).
  const underBar = Math.max(
    0,
    -Number.parseFloat(getComputedStyle(pane).marginTop) || 0,
  );
  return pane.getBoundingClientRect().top + underBar;
}

// Motion follows the reader's own setting: a reduced-motion preference makes
// every reveal on this page an instant jump rather than a glide.
function scrollBehavior(): ScrollBehavior {
  const reduced = globalThis.matchMedia?.(
    "(prefers-reduced-motion: reduce)",
  ).matches;
  return reduced ? "auto" : "smooth";
}
