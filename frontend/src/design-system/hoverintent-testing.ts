// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { HoverIntent, useHoverIntent } from "./hoverintent";

const INERT: HoverIntent = {
  onPointerEnter: () => {},
  onPointerLeave: () => {},
};

let armed = false;

/**
 * Let a pointer open hover-intent triggers for the rest of this case.
 *
 * vitest.setup.ts wraps the hook with `inertUnlessArmed` and disarms after
 * every case, so only a case whose subject IS hover pays for the real timer.
 */
export function armHoverIntent(): void {
  armed = true;
}

export function disarmHoverIntent(): void {
  armed = false;
}

// The real hook always runs, so arming mid-render never changes the hook
// order React holds a mounted trigger to; only the handlers it returns swap.
export function inertUnlessArmed(
  real: typeof useHoverIntent,
): typeof useHoverIntent {
  return (onOpen, onClose, options) => {
    const intent = real(onOpen, onClose, options);
    return armed ? intent : INERT;
  };
}
