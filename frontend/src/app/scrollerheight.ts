// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect } from "react";

/**
 * Publishes the scroller's height as `--scrollerH` on the scroller, for a
 * sticky pane that caps itself to what the scroller shows. A shell banner
 * shortens the scroller, and a stylesheet's viewport units cannot see that.
 * `layout` names which of the shell's scrollers is mounted, so a switch
 * between them observes the new one.
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
    const publish = () =>
      element.style.setProperty("--scrollerH", `${element.clientHeight}px`);
    publish();
    const observer = new ResizeObserver(publish);
    observer.observe(element);
    return () => observer.disconnect();
  }, [scroller, layout]);
}
