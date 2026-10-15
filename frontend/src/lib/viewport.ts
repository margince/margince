// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";

// The phone breakpoint, spelled once for the code that has to KNOW it rather
// than merely be laid out by it: at this width the sidebar is a bottom bar of
// three destinations, the agent and More, which changes what the panel renders
// and where it is measured from, not only how it looks. The
// `@media (max-width: 700px)` block in app/shell.css is the other half of the
// same rule — a stylesheet cannot read a TypeScript constant, so that block
// cites this file and the two are changed together.
export const PHONE_MAX_WIDTH = 700;

// The width at which the LAYOUT turns: the tree's own narrow breakpoint, which
// thirteen stylesheets already spell as `@media (max-width: 720px)` — the modal,
// the record head, the page zones and the worklist row among them. It is here
// for the code that must AGREE with that fold rather than merely be laid out by
// it: the worklist's set-aside verbs are drawn as buttons above this width and
// answered by a swipe below it, and a component keying that on a different
// number would draw both at the widths between the two.
//
// A stylesheet cannot read a TypeScript constant, so those blocks and this line
// are changed together, the same pairing PHONE_MAX_WIDTH keeps with
// app/shell.css.
export const FOLD_MAX_WIDTH = 720;

// The width at which a LIST HEADER stops fitting its verbs. Below it the row of
// actions folds into one overflow menu (design-system/listsurface.tsx). It is
// wider than the phone breakpoint on purpose: nothing about the shell changes
// here, only how much a card's own header row can hold, and the header ran out
// of room a long way above the width at which the sidebar becomes a bar.
export const NARROW_MAX_WIDTH = 1100;

function widthQuery(maxWidth: number): MediaQueryList | undefined {
  return globalThis.matchMedia?.(`(max-width: ${maxWidth}px)`);
}

/**
 * Whether the viewport is at or under `maxWidth`.
 *
 * Subscribed rather than measured once: a window is resized and a phone is
 * rotated while the app is open, and chrome that read the width at mount would
 * keep the other width's arrangement for the rest of the session.
 *
 * `matchMedia` is absent in some embedded contexts, and a missing media query is
 * a DEFAULT rather than an error — the answer is then "not narrow", which is
 * the arrangement that works at any width.
 */
export function useViewportUnder(maxWidth: number): boolean {
  const [under, setUnder] = useState(
    () => widthQuery(maxWidth)?.matches ?? false,
  );
  useEffect(() => {
    const query = widthQuery(maxWidth);
    if (!query) {
      return;
    }
    // Read again on subscribe: the first render can happen before the window
    // has settled at the size it ends up at, and the listener below only ever
    // reports a CHANGE from whatever the query matched then.
    setUnder(query.matches);
    const listen = () => setUnder(query.matches);
    query.addEventListener("change", listen);
    return () => query.removeEventListener("change", listen);
  }, [maxWidth]);
  return under;
}

/** Whether the viewport is at phone width, where the sidebar is a bottom bar. */
export function usePhoneViewport(): boolean {
  return useViewportUnder(PHONE_MAX_WIDTH);
}

/** Whether the layout has folded to its narrow arrangement. */
export function useFoldedViewport(): boolean {
  return useViewportUnder(FOLD_MAX_WIDTH);
}

/** Whether a card's header has to fold its row of verbs into one menu. */
export function useNarrowViewport(): boolean {
  return useViewportUnder(NARROW_MAX_WIDTH);
}
