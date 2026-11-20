// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useRef } from "react";

/**
 * Lifts a floating element into the browser's top layer, so no stacking
 * context or z-index on the page covers it.
 *
 * The element carries `popover="manual"`: the browser shows and stacks it,
 * and the component keeps its own opening, dismissal and placement. Layers
 * stack in the order they open, so one opened from inside another stands
 * above it.
 */
export function showInTopLayer(element: HTMLElement | null): void {
  element?.showPopover();
}

/**
 * Shows an element again, above every layer opened since it was. The reset in
 * base.css keeps it displayed meanwhile, so focus and hover stay inside it.
 */
export function raiseInTopLayer(element: HTMLElement | null): void {
  element?.hidePopover();
  element?.showPopover();
}

/** `showInTopLayer` for each element mounted while `open` holds. */
export function useTopLayer(
  ref: RefObject<HTMLElement | null>,
  open: boolean,
): void {
  // The node last shown, because a panel can be replaced while `open` holds
  // and the new node needs a show of its own.
  const shown = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    const element = open ? ref.current : null;
    if (element !== null && element !== shown.current) {
      showInTopLayer(element);
    }
    shown.current = element;
  });
}
