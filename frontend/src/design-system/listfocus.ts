// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useRef } from "react";
import { useFocusHandoff } from "./focushandoff";

// A press can take its own control away. Focus then lands on the list's title,
// else the title of the section around it; a scroll box only once it is named.
function listLanding(from: Element | null): HTMLElement | null {
  const list = from?.closest(".lt");
  const section = from?.closest("section[aria-labelledby]");
  const sectionTitle = section?.getAttribute("aria-labelledby");
  return (
    list?.querySelector<HTMLElement>(".lt-title") ??
    (sectionTitle ? document.getElementById(sectionTitle) : null) ??
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
