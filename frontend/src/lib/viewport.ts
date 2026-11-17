// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";

// The phone breakpoint, for the code that has to know it rather than be laid out
// by it. At this width the sidebar is a bottom bar of three destinations, the
// agent and More. That changes what the panel renders and where it is measured.
// Stylesheets spell it in their own `@media (max-width: 700px)` blocks, and
// census/rules/viewport.census.ts fails a phone block that spells another
// number.
export const PHONE_MAX_WIDTH = 700;

// The width at which the layout folds, for code that must agree with the
// stylesheets' fold. The worklist draws its set-aside verbs as buttons above it
// and answers a swipe below it, so another number would draw both in between.
// A stylesheet cannot read this constant, so census/rules/viewport.census.ts
// fails a fold block that spells another number.
export const FOLD_MAX_WIDTH = 720;

// The width at which a LIST HEADER stops fitting its verbs. Below it the row of
// actions folds into one overflow menu (design-system/listsurface.tsx). It is
// wider than the phone breakpoint on purpose: nothing about the shell changes
// here, only how much a card's own header row can hold, and the header ran out
// of room a long way above the width at which the sidebar becomes a bar.
export const NARROW_MAX_WIDTH = 1100;

/** The media query the hooks below ask for `maxWidth`. */
export function widthMedia(maxWidth: number): string {
  return `(max-width: ${maxWidth}px)`;
}

export const COARSE_POINTER = "(pointer: coarse)";

/**
 * Subscribed rather than measured once: a window is resized and a phone is
 * rotated while the app is open. Where `matchMedia` is absent the answer is no.
 */
export function useMediaMatch(media: string): boolean {
  const [matches, setMatches] = useState(
    () => globalThis.matchMedia?.(media).matches ?? false,
  );
  useEffect(() => {
    const query = globalThis.matchMedia?.(media);
    if (!query) {
      return;
    }
    // Read again on subscribe: the first render can happen before the window
    // has settled at the size it ends up at, and the listener below only ever
    // reports a CHANGE from whatever the query matched then.
    setMatches(query.matches);
    const listen = () => setMatches(query.matches);
    query.addEventListener("change", listen);
    return () => query.removeEventListener("change", listen);
  }, [media]);
  return matches;
}

/** Whether the viewport is at or under `maxWidth`. */
export function useViewportUnder(maxWidth: number): boolean {
  return useMediaMatch(widthMedia(maxWidth));
}

/** Whether the reader points with a finger, so the device is a phone or tablet. */
export function useCoarsePointer(): boolean {
  return useMediaMatch(COARSE_POINTER);
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
