// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect, useRef } from "react";

// The press that opened a page unmounted its control and dropped focus on <body>;
// the page title (tabIndex -1) takes it so a screen reader says where it landed.
export function useArrivalFocus<
  Target extends HTMLElement,
>(): RefObject<Target | null> {
  const target = useRef<Target>(null);
  useEffect(() => {
    const active = document.activeElement;
    if (active === null || active === document.body) {
      target.current?.focus({ preventScroll: true });
    }
  }, []);
  return target;
}
