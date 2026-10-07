// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect, useLayoutEffect, useState } from "react";

type ScrollAxis = "inline" | "block";

/**
 * What the region is called: a phrase of the caller's own, or the id of a
 * heading already on the box, so a reader does not hear the same words twice.
 */
export type ScrollRegionName = string | Readonly<{ labelledBy: string }>;

/** Spread onto the scrolling box. Empty while it has nothing hidden to reach. */
type ScrollRegion = Readonly<{
  tabIndex?: 0;
  role?: "region";
  "aria-label"?: string;
  "aria-labelledby"?: string;
}>;

/** Whether a box is holding more than it is showing along `axis`. */
function overflows(element: HTMLElement | null, axis: ScrollAxis): boolean {
  if (element === null) {
    return false;
  }
  if (axis === "block") {
    return element.scrollHeight - element.clientHeight > 1;
  }
  return element.scrollWidth - element.clientWidth > 1;
}

/**
 * Make a box that scrolls reachable, and only then: sideways, or downward for a
 * `block` box such as a popover capped to the room below it.
 *
 * A region holding content past its edge is content pointer users can
 * drag to and keyboard users cannot reach at all, so it takes a tab stop and
 * announces itself by name. It takes neither while it fits: a tab stop in front
 * of every table in the product, most of which fit, is a cost every keyboard
 * reader pays for the few that do not. That is the same bargain
 * `useTruncationTooltip` strikes for a string that fits its row.
 */
export function useScrollRegion(
  box: RefObject<HTMLElement | null>,
  name: ScrollRegionName,
  axis: ScrollAxis = "inline",
): ScrollRegion {
  const [scrolls, setScrolls] = useState(false);
  const [watched, setWatched] = useState<HTMLElement | null>(null);
  // Measured after every render rather than when the rows change: the answer
  // moves for reasons this hook never sees — a column the reader dragged, a
  // cell whose badge arrived — and re-reading it is two property reads. Setting
  // either answer twice is a no-op, so this cannot loop. The element goes into
  // state as well, so a box that unmounts and comes back (a list switching
  // between a board and a table) is re-watched rather than leaving the observer
  // below holding a node that is no longer on the page.
  useLayoutEffect(() => {
    setScrolls(overflows(box.current, axis));
    setWatched(box.current);
  });
  // A window resize is only one of the ways the box changes size, and the least
  // interesting one: the sidebar collapsing, a rail opening beside the table,
  // a settings card that is 720px on one route and full width on the next all
  // move the edge without the window moving at all. So the BOX is watched, and
  // everything inside it too — a table or a row that grew is the other half of
  // the same question.
  useEffect(() => {
    // Measured once wherever the observer is unavailable (jsdom): the answer is
    // still right for the render that just happened, it simply stops following
    // a resize.
    if (!watched || typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(() =>
      setScrolls(overflows(watched, axis)),
    );
    observer.observe(watched);
    for (const child of watched.children) {
      observer.observe(child);
    }
    return () => observer.disconnect();
  }, [watched, axis]);
  if (!scrolls) {
    return {};
  }
  return typeof name === "string"
    ? { tabIndex: 0, role: "region", "aria-label": name }
    : { tabIndex: 0, role: "region", "aria-labelledby": name.labelledBy };
}
