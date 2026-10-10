// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type SyntheticEvent, useEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";

type Press<E extends SyntheticEvent> = (event: E) => void;

// A press that lands on a control already waiting for its own answer. Both
// halves are load bearing: `preventDefault` is what stops a `type="submit"`
// button posting the form a second time (a plain early return does not — the
// browser submits on the click, not on the handler), and `stopPropagation`
// stops a clickable row underneath treating the press as a click on itself.
export function swallowWhileBusy(event: SyntheticEvent) {
  event.preventDefault();
  event.stopPropagation();
}

// Refuses the second press of a double press until the first one's write has
// been drawn and has ended. `pending` is published a task after the press, so
// a ref set inside the press is the only guard no render can be late for.
//
// With `pending` the latch holds until one more task has passed and a forced
// commit has seen the write drawn, or none. That task queues behind the one a
// mutation uses to publish `pending`. Without `pending` the latch lets go at
// the next commit. Works for a click and for a form's submit alike.
export function useSinglePress(
  pending?: boolean,
): <E extends SyntheticEvent>(handler: Press<E> | undefined) => Press<E> {
  const pressed = useRef(false);
  const drawn = useRef(false);
  const waited = useRef(false);
  // Whether `pending` was passed when the press landed. A reused control can
  // be handed one later, and that must not hold a press made without it.
  const held = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  // The tick is never read. It guarantees a commit after the wait, so the
  // release below runs even for a press that changed no state at all.
  const [, settle] = useState(0);
  useEffect(() => {
    if (held.current && pending === true) {
      drawn.current = true;
    } else if (!held.current || drawn.current || waited.current) {
      drawn.current = false;
      waited.current = false;
      pressed.current = false;
    }
  });
  useEffect(() => () => clearTimeout(timer.current), []);
  return (handler) => (event) => {
    if (pressed.current) {
      swallowWhileBusy(event);
      return;
    }
    pressed.current = true;
    held.current = pending !== undefined;
    try {
      handler?.(event);
    } finally {
      if (!held.current) {
        queueMicrotask(() => settle((tick) => tick + 1));
      } else {
        clearTimeout(timer.current);
        timer.current = setTimeout(() => {
          waited.current = true;
          // Synchronous, so the update publishing `pending` renders in the same
          // pass and the release cannot land a render early.
          flushSync(() => settle((tick) => tick + 1));
        }, 0);
      }
    }
  };
}
