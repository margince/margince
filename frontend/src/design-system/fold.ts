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

/** When code closes a body that holds focus, focus goes to the control naming it in
 * `aria-controls`; `inert` would drop it to the page. */
export function useFocusHandBack(body: string, open: boolean) {
  const was = useRef(open);
  useLayoutEffect(() => {
    const closed = was.current && !open;
    was.current = open;
    const region = document.getElementById(body);
    if (
      !closed ||
      region === null ||
      !region.contains(document.activeElement)
    ) {
      return;
    }
    const control = [...document.querySelectorAll("[aria-controls]")].find(
      (each) => each.getAttribute("aria-controls") === body,
    );
    if (control instanceof HTMLElement) {
      control.focus();
    }
  }, [body, open]);
}

// While open the box follows its content: through the open motion, and to each
// new height a late read brings. A new width alone only re-measures.
function follow(
  element: HTMLElement,
  ends: FoldEnds,
  current: () => boolean,
): () => void {
  let rest = 0;
  const measure = () => {
    rest = ends(element).open;
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
    if (Math.abs(rest - before) < 0.5) {
      return;
    }
    moveTo(element, before, rest);
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
