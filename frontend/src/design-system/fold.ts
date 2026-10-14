// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useRef } from "react";
import { usePresence } from "./presence";

// Height cannot move to or from `auto`, so the box is held at measured pixels
// while `[data-folding]` gives it the transition. The stylesheet owns the time,
// the curve and the reduced-motion answer.

/** The box's border-box height shut and open, measured while it is open. */
export type FoldEnds = (box: HTMLElement) => Readonly<{
  shut: number;
  open: number;
}>;

const FOLDING = "data-folding";

/**
 * Animate `box` between its shut and open heights as `open` flips. It returns
 * whether the content is still on screen: true while open and through the close.
 *
 * A flip mid-motion turns the box from where it is. With nothing that moves
 * (reduced motion, jsdom) the box lands at once.
 */
export function useFold(
  box: RefObject<HTMLElement | null>,
  open: boolean,
  ends: FoldEnds,
): boolean {
  const last = useRef(open);
  const run = useRef(0);
  // Declared before `usePresence`, whose effect asks the box what is moving:
  // the transition must exist by then, or the close ends before it starts.
  useLayoutEffect(() => {
    const element = box.current;
    if (last.current === open) {
      return;
    }
    last.current = open;
    run.current += 1;
    if (element === null || typeof element.getAnimations !== "function") {
      return;
    }
    const token = run.current;
    const at = ends(element);
    if (!element.hasAttribute(FOLDING)) {
      element.style.setProperty("height", `${open ? at.shut : at.open}px`);
      element.setAttribute(FOLDING, "");
      // A layout read commits the start height, so the next one transitions.
      element.getBoundingClientRect();
    }
    element.style.setProperty("height", `${open ? at.open : at.shut}px`);
    if (!open) {
      return;
    }
    const growing = element.getAnimations().filter(isHeight);
    void Promise.allSettled(growing.map((each) => each.finished)).then(() => {
      if (run.current === token) {
        release(element);
      }
    });
  }, [box, ends, open]);
  const { mounted } = usePresence({ open, element: box });
  useLayoutEffect(() => {
    if (!mounted && box.current !== null) {
      release(box.current);
    }
  }, [box, mounted]);
  return mounted;
}

function isHeight(animation: Animation): boolean {
  return (
    "transitionProperty" in animation &&
    animation.transitionProperty === "height"
  );
}

function release(element: HTMLElement) {
  element.style.removeProperty("height");
  element.removeAttribute(FOLDING);
}
