// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Marks the one element the pointer is over with `data-pointer-over`.
 *
 * app.css lights the scrollbar of that element only. The rule used to be
 * `*:hover:not(:has(:hover))`. Chromium recomputes the style of every element
 * on each hover change while any `:has()` holds `:hover`. On a board column of
 * 500 cards a pointer resting on the column cost hundreds of milliseconds per
 * scrolled frame.
 *
 * A `pointerover` event targets the deepest element under the pointer, which
 * is the element that rule picked out. Moving one attribute restyles two
 * elements, the old target and the new one.
 */

export const POINTER_OVER_ATTRIBUTE = "data-pointer-over";

export function startPointerTarget(doc: Document = document): () => void {
  let current: Element | null = null;
  const mark = (next: Element | null) => {
    if (next === current) {
      return;
    }
    current?.removeAttribute(POINTER_OVER_ATTRIBUTE);
    next?.setAttribute(POINTER_OVER_ATTRIBUTE, "");
    current = next;
  };
  const over = (event: Event) => {
    mark(event.target instanceof Element ? event.target : null);
  };
  // A pointer that leaves the window is over nothing, as `:hover` holds.
  const out = (event: Event) => {
    if ((event as PointerEvent).relatedTarget === null) {
      mark(null);
    }
  };
  doc.addEventListener("pointerover", over, { passive: true });
  doc.addEventListener("pointerout", out, { passive: true });
  return () => {
    doc.removeEventListener("pointerover", over);
    doc.removeEventListener("pointerout", out);
    mark(null);
  };
}
