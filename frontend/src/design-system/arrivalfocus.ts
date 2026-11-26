// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect, useRef } from "react";
import { parseHash, type Route, watchAddress } from "../lib/router";

// The press that opened a page unmounted its control and dropped focus on
// <body>. The page title (tabIndex -1) takes it, so a screen reader says where
// it landed. A mount no press caused, such as a first load, sets `when` false.
export function useArrivalFocus<Target extends HTMLElement>(
  when = true,
): RefObject<Target | null> {
  const target = useRef<Target>(null);
  useEffect(() => {
    if (when) {
      takeDroppedFocus(target.current);
    }
  }, [when]);
  return target;
}

// The page a press is opening, set just before it navigates there. A page that
// many places open cannot tell which mount a press caused, so the press says
// so. It holds while the reader stays on that page and has not pressed or typed.
// A title that mounts again there once the record loads still takes the focus.
// After that, the reader decides where focus goes.
let awaited: Route | null = null;
let stopWatching: (() => void) | null = null;

/** Marks the page `route` names as opened by a press, for its title to take focus. */
export function awaitArrival(route: Route): void {
  forgetArrival();
  awaited = route;
  const stopAddress = watchAddress(() => {
    if (!onAwaitedPage()) {
      forgetArrival();
    }
  });
  const options = { capture: true, once: true };
  globalThis.addEventListener("pointerdown", forgetArrival, options);
  globalThis.addEventListener("keydown", forgetArrival, options);
  stopWatching = () => {
    stopAddress();
    globalThis.removeEventListener("pointerdown", forgetArrival, options);
    globalThis.removeEventListener("keydown", forgetArrival, options);
  };
}

function forgetArrival(): void {
  stopWatching?.();
  stopWatching = null;
  awaited = null;
}

/**
 * `useArrivalFocus` for a page many places open: the title takes focus on a
 * mount `awaitArrival` named, and on no other mount.
 */
export function useAwaitedArrivalFocus<
  Target extends HTMLElement,
>(): RefObject<Target | null> {
  const target = useRef<Target>(null);
  useEffect(() => {
    if (onAwaitedPage()) {
      takeDroppedFocus(target.current);
    }
  }, []);
  return target;
}

// A page may add segments of its own (a tab), so only the ones the press named
// must match.
function onAwaitedPage(): boolean {
  const route = awaited;
  if (route === null) {
    return false;
  }
  const here = parseHash(globalThis.location.hash);
  return (["screen", "id", "id2", "id3"] as const).every(
    (segment) =>
      route[segment] === undefined || route[segment] === here[segment],
  );
}

// Focus the reader put in any other place stays there.
function takeDroppedFocus(target: HTMLElement | null): void {
  const active = document.activeElement;
  if (active === null || active === document.body) {
    target?.focus({ preventScroll: true });
  }
}
