// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { vi } from "vitest";
import type { useHoverIntent } from "./hoverintent";

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

/**
 * Take the clock the hook reasons with. `performance` is faked alongside the
 * timers, or the poll measures real elapsed time against simulated time. The
 * caller hands it back with `vi.useRealTimers()`.
 */
export function takeHoverClock(): void {
  vi.useFakeTimers({
    toFake: [
      "setTimeout",
      "clearTimeout",
      "setInterval",
      "clearInterval",
      "performance",
    ],
  });
}

// Asked when the pointer arrives rather than at render, so arming after mount
// takes effect at once. Leave always reaches the real hook: it is a no-op with
// nothing pending, and a poll armed before a disarm still has to stop.
export function inertUnlessArmed(
  real: typeof useHoverIntent,
): typeof useHoverIntent {
  return (onOpen, onClose, options) => {
    const intent = real(onOpen, onClose, options);
    return {
      onPointerEnter: () => {
        if (armed) {
          intent.onPointerEnter();
        }
      },
      onPointerLeave: intent.onPointerLeave,
    };
  };
}
