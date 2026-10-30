// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, type RefObject, useLayoutEffect, useRef } from "react";
import { fingerprint, markRevealed, revealedBefore } from "./aireveal.seen";
import "./aireveal.css";

// The animations that end a reveal: the colour drain, or the fade that stands
// in for it under reduced motion. The wipe ends first and must not cut it short.
const LAST_ANIMATION = new Set(["aireveal-color", "aireveal-fade"]);

// Past this many screens tall the block arrives at rest: the reveal repaints
// the whole block each frame, and a reader sees one screen of it anyway.
const MAX_SCREENS = 1.5;

/**
 * Plays the arrival of machine-written `text` on `ref`'s element, under one
 * rule: the first time this reader has that text on screen, and never again.
 *
 * - Text this browser already revealed (a reopened draft, a cached brief, the
 *   same answer asked twice) stands still. The record is a fingerprint per
 *   text, kept for the signed-in seat (`aireveal.seen.ts`).
 * - Text that lands off screen waits, hidden, and plays when it scrolls in,
 *   so a reveal nobody watched is not spent.
 *
 * The run is a data attribute rather than a class: React owns `className` and
 * rewrites it on a render, while it never touches an attribute it did not set.
 */
export function useAiReveal(
  ref: RefObject<HTMLElement | null>,
  text: string | null | undefined,
) {
  useLayoutEffect(() => {
    const el = ref.current;
    if (!text || !el) {
      return;
    }
    const print = fingerprint(text);
    if (revealedBefore(print)) {
      return;
    }
    const finish = (event: AnimationEvent) => {
      if (event.target === el && LAST_ANIMATION.has(event.animationName)) {
        el.removeAttribute("data-ai-reveal");
      }
    };
    const play = () => {
      markRevealed(print);
      if (
        el.getBoundingClientRect().height >
        window.innerHeight * MAX_SCREENS
      ) {
        el.removeAttribute("data-ai-reveal");
        return;
      }
      el.setAttribute("data-ai-reveal", "run");
    };
    el.addEventListener("animationend", finish);
    if (typeof IntersectionObserver === "undefined") {
      play();
      return () => el.removeEventListener("animationend", finish);
    }
    el.setAttribute("data-ai-reveal", "wait");
    const onScreen = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        onScreen.disconnect();
        play();
      }
    });
    onScreen.observe(el);
    return () => {
      onScreen.disconnect();
      el.removeEventListener("animationend", finish);
      // A reveal cut short by new text or an unmount leaves the words at rest.
      el.removeAttribute("data-ai-reveal");
    };
  }, [text, ref]);
}

/**
 * AiReveal is the arrival of words a model wrote: they wipe in under a band of
 * moving indigo and green, and the colour drains into the page's own ink. It
 * follows `AiPending`, whose speck rows hold the place while the model works,
 * and plays only the first time its `text` is on this reader's screen.
 */
export function AiReveal({
  text,
  children,
}: Readonly<{
  /**
   * The words the block shows, as plain text: what "seen before" is about.
   * Without words there is nothing to reveal, and the block stands still.
   */
  text: string | null | undefined;
  children: ReactNode;
}>) {
  const block = useRef<HTMLDivElement>(null);
  useAiReveal(block, text);
  return <div ref={block}>{children}</div>;
}
