// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type MouseEvent as ReactMouseEvent,
  useEffect,
  useRef,
  useState,
} from "react";

type Press = (event: ReactMouseEvent<HTMLButtonElement>) => void;

// A press that lands on a control already waiting for its own answer. Both
// halves are load bearing: `preventDefault` is what stops a `type="submit"`
// button posting the form a second time (a plain early return does not — the
// browser submits on the click, not on the handler), and `stopPropagation`
// stops a clickable row underneath treating the press as a click on itself.
export function swallowWhileBusy(event: ReactMouseEvent<HTMLButtonElement>) {
  event.preventDefault();
  event.stopPropagation();
}

/**
 * The second press of a double press, refused before the first one's write has
 * said it is out.
 *
 * `pending` answers a RENDER late and cannot be the whole guard: the write
 * starts inside the handler, the state saying so is dispatched from a later
 * task, and the commit that finally draws the control busy lands after that. A
 * press arriving in the gap finds the caller's own handler still on the button
 * and submits a second time. The gap is too narrow to hit on an idle machine
 * and opens wide on a blocked main thread — which is exactly when a reader who
 * saw nothing happen presses again.
 *
 * So the refusal is a ref, set inside the press itself where no render can be
 * late, and released one settled commit later. By then `pending` either says a
 * write is out, and the busy swallow holds the door from there, or nothing was
 * started and the control has to work again. The release is scheduled from
 * inside the press so it queues BEHIND whatever the handler started, and from
 * a `finally` so a handler that throws cannot leave the control dead.
 */
export function useSinglePress(): (handler: Press | undefined) => Press {
  const pressed = useRef(false);
  // The tick is never read. It exists to guarantee a commit, so the release
  // below runs even for a handler that changed no state at all — without it a
  // control whose press starts no write would stay latched for good.
  const [, settle] = useState(0);
  useEffect(() => {
    pressed.current = false;
  });
  return (handler) => (event) => {
    if (pressed.current) {
      swallowWhileBusy(event);
      return;
    }
    pressed.current = true;
    try {
      handler?.(event);
    } finally {
      queueMicrotask(() => settle((tick) => tick + 1));
    }
  };
}
