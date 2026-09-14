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
 * section is not drawn: the deal strip is withheld in overlay, and every other
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
