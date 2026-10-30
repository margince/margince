// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ReactNode,
  type RefObject,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import "./aireveal.css";

// The animations that end a reveal: the colour drain, or the fade that stands
// in for it under reduced motion. The wipe ends first and must not cut it short.
const LAST_ANIMATION = new Set(["aireveal-color", "aireveal-fade"]);

/**
 * Plays the arrival of machine-written words on `ref`'s element each time
 * `cue` changes to a value other than null or undefined.
 *
 * The run is a data attribute rather than a class: React owns `className` and
 * rewrites it on a render, while it never touches an attribute it did not set,
 * so a re-render in the middle of the arrival cannot end it.
 */
export function useAiReveal(
  ref: RefObject<HTMLElement | null>,
  cue: number | null | undefined,
) {
  useLayoutEffect(() => {
    const el = ref.current;
    if (cue == null || !el) {
      return;
    }
    const finish = (event: AnimationEvent) => {
      if (event.target === el && LAST_ANIMATION.has(event.animationName)) {
        el.removeAttribute("data-ai-reveal");
      }
    };
    el.removeAttribute("data-ai-reveal");
    // Reading layout between the two writes restarts an arrival in flight.
    el.getBoundingClientRect();
    el.setAttribute("data-ai-reveal", "run");
    el.addEventListener("animationend", finish);
    return () => el.removeEventListener("animationend", finish);
  }, [cue, ref]);
}

/**
 * A cue for `useAiReveal` that moves only when a wait this component saw ends.
 * Null until then, so an answer that was already there when the surface
 * opened (a cached read, a tab visited twice) stands still.
 */
export function useArrivalCue(waiting: boolean): number | null {
  const [seen, setSeen] = useState<{ waiting: boolean; cue: number | null }>({
    waiting,
    cue: null,
  });
  if (seen.waiting !== waiting) {
    const cue = waiting ? seen.cue : (seen.cue ?? 0) + 1;
    setSeen({ waiting, cue });
    return cue;
  }
  return seen.cue;
}

/**
 * AiReveal is the arrival of words a model wrote: they wipe in under a band of
 * moving indigo and green, and the colour drains into the page's own ink. It
 * follows `AiPending`, whose speck rows hold the place while the model works.
 * At rest it is a plain block and draws nothing of its own.
 */
export function AiReveal({
  cue,
  children,
}: Readonly<{
  /** Plays on mount when set, and again on every change (a rewrite). */
  cue: number | null | undefined;
  children: ReactNode;
}>) {
  const block = useRef<HTMLDivElement>(null);
  useAiReveal(block, cue);
  return <div ref={block}>{children}</div>;
}
