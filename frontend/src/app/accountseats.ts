// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useRef, useState } from "react";

/**
 * What a row needs to be a menu item. It holds the row's roving tabstop, the
 * handle the menu moves focus with, and the callback that records focus.
 */
export type RowSeat = Readonly<{
  ref: (element: HTMLElement | null) => void;
  tabIndex: number;
  onFocus: () => void;
}>;

/** A disabled control cannot hold focus, so the roving walk steps over it rather
 *  than landing the reader on a row nothing happens on. */
function isSeatFocusable(element: HTMLElement | null): element is HTMLElement {
  return (
    element !== null &&
    !(element instanceof HTMLButtonElement && element.disabled)
  );
}

/**
 * The next seat a reader can actually stand on, wrapping at both ends.
 *
 * `count * count` keeps the modulus positive for a backwards walk; the loop
 * gives up after one full lap, which is the case where every other row is
 * disabled and the reader stays where they are.
 */
export function nextSeat(
  seats: readonly (HTMLElement | null)[],
  from: number,
  delta: number,
): number {
  const count = seats.length;
  for (let hop = 1; hop <= count; hop++) {
    const at = (from + delta * hop + count * count) % count;
    if (isSeatFocusable(seats[at] ?? null)) {
      return at;
    }
  }
  return from;
}

/**
 * One roving tabstop over a menu's `count` rows, for the account menu and for
 * every menu that flies out of it: Up and Down step, Home and End go to the ends.
 */
export function useRovingSeats(count: number) {
  const rows = useRef<(HTMLElement | null)[]>([]);
  const [active, setActive] = useState(0);

  const move = useCallback((at: number) => {
    setActive(at);
    rows.current[at]?.focus();
  }, []);

  /** Walks for a key it owns, and says whether it did. */
  const walk = (key: string): boolean => {
    switch (key) {
      case "ArrowDown":
        move(nextSeat(rows.current, active, 1));
        return true;
      case "ArrowUp":
        move(nextSeat(rows.current, active, -1));
        return true;
      case "Home":
        move(nextSeat(rows.current, count - 1, 1));
        return true;
      case "End":
        move(nextSeat(rows.current, 0, -1));
        return true;
      default:
        return false;
    }
  };

  const seat = (index: number): RowSeat => ({
    ref: (element) => {
      rows.current[index] = element;
    },
    tabIndex: active === index ? 0 : -1,
    onFocus: () => setActive(index),
  });

  return { seat, walk, move };
}
