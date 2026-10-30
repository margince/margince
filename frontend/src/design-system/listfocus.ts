// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useRef } from "react";
import { useFocusHandoff } from "./focushandoff";

// A press can take its own control away. Focus then lands on the first named
// thing that stays: the list's title, its Panel's title, its search box.
function listLanding(from: Element | null): HTMLElement | null {
  const list = from?.closest(".lt");
  const panel = from?.closest(".panel");
  return (
    list?.querySelector<HTMLElement>(".lt-title") ??
    panel?.querySelector<HTMLElement>(":scope > .panel-head > .panel-title") ??
    list?.querySelector<HTMLElement>(".lt-search-input") ??
    list?.querySelector<HTMLElement>(".lt-scroll[role]") ??
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
