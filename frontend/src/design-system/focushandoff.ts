// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useRef } from "react";

// A write that runs at once can unmount its own control and drop focus to
// <body>. The landing is read at unmount, so a re-render never moves focus.
export function useFocusHandoff(
  source: RefObject<HTMLElement | null>,
  landing: () => HTMLElement | null,
): void {
  const latest = useRef(landing);
  // Copied each commit: the source's ref may already be detached at unmount.
  const held = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    latest.current = landing;
    held.current = source.current;
  });
  useLayoutEffect(
    () => () => {
      if (held.current?.contains(document.activeElement)) {
        latest.current()?.focus();
      }
    },
    [],
  );
}
