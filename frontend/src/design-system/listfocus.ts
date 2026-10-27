// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useLayoutEffect, useRef } from "react";
import { useFocusHandoff } from "./focushandoff";

// A press can take its own control away. Focus then lands on the list's title,
// or on its named scroll region when the list has no title.
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
