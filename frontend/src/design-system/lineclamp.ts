// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useLayoutEffect, useState } from "react";

/**
 * Whether a line-clamped box hides any of its text, measured at its own width.
 * A guess by length leaves clipped text with no control to reach it. Pass the
 * box through the returned ref. `open` is the unclamped state, where nothing is
 * measured, so the control that closes it again stays. `content` re-measures
 * when the text changes.
 */
export function useLineClamped(
  open: boolean,
  content: unknown,
): [(box: HTMLElement | null) => void, boolean] {
  const [box, setBox] = useState<HTMLElement | null>(null);
  const [clipped, setClipped] = useState(false);
  useLayoutEffect(() => {
    if (!box || open || !content) {
      return;
    }
    let live = true;
    const measure = () => {
      if (live) setClipped(box.scrollHeight > box.clientHeight + 1);
    };
    measure();
    // A late web font rewraps the text inside a box whose height the clamp
    // holds, so no resize reports it. `ready` never rejects.
    document.fonts?.ready.then(measure);
    // A width change can start or stop the clipping. jsdom has no observer.
    const observer =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(measure);
    observer?.observe(box);
    return () => {
      live = false;
      observer?.disconnect();
    };
  }, [box, open, content]);
  return [setBox, clipped];
}
