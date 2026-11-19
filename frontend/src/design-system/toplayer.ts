// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect } from "react";

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

/** `showInTopLayer` for an element mounted while `open` holds. */
export function useTopLayer(
  ref: RefObject<HTMLElement | null>,
  open: boolean,
): void {
  useLayoutEffect(() => {
    if (open) showInTopLayer(ref.current);
  }, [ref, open]);
}
