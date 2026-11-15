// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Marks the deepest element under the pointer with `data-pointer-over`, so
 * app.css can light that one scrollbar without `:has(:hover)`. Chromium
 * restyles every element on each hover change while any `:has()` holds
 * `:hover`. Moving this attribute restyles two elements.
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
