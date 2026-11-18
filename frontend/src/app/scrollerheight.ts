// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect } from "react";

/**
 * Publishes the scroller's height (`--scrollerH`, for a sticky pane) and, on
 * the column, its scrollbar's width (`--scrollerGutter`, for the top bar). A
 * banner shortens the scroller, which viewport units cannot see. `layout`
 * names the mounted scroller, so a switch between them observes the new one.
 */
export function usePublishedScrollerHeight(
  scroller: RefObject<HTMLElement | null>,
  layout: string,
): void {
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only dep — `layout` is not read in the body; a new layout is a new scroller element, and re-running the effect observes it.
  useLayoutEffect(() => {
    const element = scroller.current;
    if (element === null || typeof ResizeObserver === "undefined") {
      return;
    }
    const publish = () => {
      element.style.setProperty("--scrollerH", `${element.clientHeight}px`);
      const edges = getComputedStyle(element);
      const gutter =
        element.offsetWidth -
        element.clientWidth -
        Number.parseFloat(edges.borderLeftWidth) -
        Number.parseFloat(edges.borderRightWidth);
      element.parentElement?.style.setProperty(
        "--scrollerGutter",
        `${Math.max(0, gutter)}px`,
      );
    };
    publish();
    const observer = new ResizeObserver(publish);
    observer.observe(element);
    return () => observer.disconnect();
  }, [scroller, layout]);
}
