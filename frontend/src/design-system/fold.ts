// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useRef } from "react";
import { usePresence } from "./presence";

// Height cannot move to or from `auto`: the box is held at measured pixels while
// `[data-folding]` moves it. The stylesheet owns the time, curve and reduced motion.

/** The box's border-box height shut and open, measured while it is open. */
export type FoldEnds = (box: HTMLElement) => Readonly<{
  shut: number;
  open: number;
}>;

const FOLDING = "data-folding";

/** Moves `box` between its shut and open heights, and with its content while open.
 * Returns whether the content is on screen: while open and through the close. */
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
    if (element === null || !moves(element)) {
      return;
    }
    const at = ends(element);
    moveTo(element, open ? at.shut : at.open, open ? at.open : at.shut);
  }, [box, ends, open]);
  useLayoutEffect(() => {
    const element = box.current;
    if (!open || element === null || !moves(element)) {
      return;
    }
    const token = run.current;
    const stop = follow(element, ends, () => run.current === token);
    return () => {
      run.current += 1;
      stop();
    };
  }, [box, ends, open]);
  const { mounted } = usePresence({ open, element: box });
  useLayoutEffect(() => {
    if (!mounted && box.current !== null) {
      release(box.current);
    }
  }, [box, mounted]);
  return mounted;
}

// While open the box tracks its content: through the open motion, and from one
// resting height to the next when a late read lands. A new width only re-measures.
function follow(
  element: HTMLElement,
  ends: FoldEnds,
  current: () => boolean,
): () => void {
  let rest = { width: 0, height: 0 };
  const measure = () => {
    rest = {
      width: element.getBoundingClientRect().width,
      height: ends(element).open,
    };
  };
  const settle = () => {
    if (!current() || !element.hasAttribute(FOLDING)) {
      return;
    }
    element.style.setProperty("height", `${ends(element).open}px`);
    const moving = element.getAnimations().filter(isHeight);
    if (moving.length > 0) {
      void Promise.allSettled(moving.map((each) => each.finished)).then(settle);
      return;
    }
    release(element);
    measure();
  };
  const resized = () => {
    if (!current()) {
      return;
    }
    if (element.hasAttribute(FOLDING)) {
      element.style.setProperty("height", `${ends(element).open}px`);
      return;
    }
    const before = rest;
    measure();
    const same = Math.abs(rest.height - before.height) < 0.5;
    if (same || rest.width !== before.width) {
      return;
    }
    moveTo(element, before.height, rest.height);
    settle();
  };
  measure();
  const observer =
    typeof ResizeObserver === "function" ? new ResizeObserver(resized) : null;
  for (const child of element.children) {
    observer?.observe(child);
  }
  settle();
  return () => observer?.disconnect();
}

function moves(element: HTMLElement): boolean {
  return typeof element.getAnimations === "function";
}

// A box already moving turns from where it is.
function moveTo(element: HTMLElement, from: number, to: number) {
  if (!element.hasAttribute(FOLDING)) {
    element.style.setProperty("height", `${from}px`);
    element.setAttribute(FOLDING, "");
    // A layout read commits the start height, so the next one transitions.
    element.getBoundingClientRect();
  }
  element.style.setProperty("height", `${to}px`);
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
