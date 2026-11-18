// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { RefObject } from "react";

/** An element's size as a `useSyncExternalStore` subscription, silent where unmeasurable. */
export function watchSize(element: RefObject<HTMLElement | null>) {
  return (changed: () => void) => {
    const node = element.current;
    if (!node || typeof ResizeObserver === "undefined") {
      return () => undefined;
    }
    const observer = new ResizeObserver(changed);
    observer.observe(node);
    // A late web font changes what sits inside without resizing the box.
    let live = true;
    const settled = () => {
      if (live) changed();
    };
    document.fonts?.ready.then(settled, settled);
    return () => {
      live = false;
      observer.disconnect();
    };
  };
}
