// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Marks the deepest element under the pointer with `data-pointer-over`, so
 * app.css can light that one scrollbar without `:has(:hover)`. Chromium
 * restyles every element on each hover change while any `:has()` holds
 * `:hover`. Moving this attribute restyles two elements.
 */

export const POINTER_OVER_ATTRIBUTE = "data-pointer-over";

export type PointerMarks = Readonly<{
  /** Points `pointerId` at `element`, or at nothing. */
  move(pointerId: number, element: Element | null): void;
  /** Points every pointer at nothing. */
  clear(): void;
}>;

/**
 * An attribute on whatever element some pointer is on. Kept per pointer: a
 * touch that comes and goes must not clear the mark a resting mouse still
 * holds, since that mouse sends no new `pointerover`.
 */
export function pointerMarks(attribute: string): PointerMarks {
  const targets = new Map<number, Element>();
  const move = (pointerId: number, next: Element | null) => {
    const previous = targets.get(pointerId) ?? null;
    if (next === previous) {
      return;
    }
    if (next === null) {
      targets.delete(pointerId);
    } else {
      targets.set(pointerId, next);
      next.setAttribute(attribute, "");
    }
    if (previous !== null && ![...targets.values()].includes(previous)) {
      previous.removeAttribute(attribute);
    }
  };
  const clear = () => {
    for (const pointerId of [...targets.keys()]) {
      move(pointerId, null);
    }
  };
  return { move, clear };
}

export function startPointerTarget(doc: Document = document): () => void {
  const marks = pointerMarks(POINTER_OVER_ATTRIBUTE);
  const over = (event: PointerEvent) => {
    const target = event.target instanceof Element ? event.target : null;
    marks.move(event.pointerId, target);
  };
  // A pointer that leaves the window, or lifts off a touch screen, is over
  // nothing, as `:hover` holds.
  const out = (event: PointerEvent) => {
    if (event.relatedTarget === null) {
      marks.move(event.pointerId, null);
    }
  };
  doc.addEventListener("pointerover", over, { passive: true });
  doc.addEventListener("pointerout", out, { passive: true });
  return () => {
    doc.removeEventListener("pointerover", over);
    doc.removeEventListener("pointerout", out);
    marks.clear();
  };
}
