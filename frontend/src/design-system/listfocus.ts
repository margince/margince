// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useRef } from "react";
import { useFocusHandoff } from "./focushandoff";

// Where a list's focus lands when a press takes its own control away: the
// list's title, or its named scroll region when the list has no title.
function listLanding(from: Element | null): HTMLElement | null {
  const list = from?.closest(".lt");
  return (
    list?.querySelector<HTMLElement>(".lt-title") ??
    list?.querySelector<HTMLElement>(".lt-scroll") ??
    null
  );
}

// The landing is found while the control is still on the page; at unmount it
// is not, so the lookup runs on every commit.
export function useListFocusHandoff(
  source: RefObject<HTMLElement | null>,
): void {
  const landing = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    landing.current = listLanding(source.current);
  });
  useFocusHandoff(source, () => landing.current);
}
