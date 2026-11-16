// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useMemo, useSyncExternalStore } from "react";
import { watchSize } from "./watchsize";

// Whether the strip fell to a second row, which lets its segments fill each
// row. Rows break on word widths alone, so the answer never undoes itself.
export function useWrapped(strip: RefObject<HTMLElement | null>): boolean {
  const subscribe = useMemo(() => watchSize(strip), [strip]);
  return useSyncExternalStore(
    subscribe,
    () => {
      const first = strip.current?.firstElementChild;
      const last = strip.current?.lastElementChild;
      return (
        first instanceof HTMLElement &&
        last instanceof HTMLElement &&
        last.offsetTop !== first.offsetTop
      );
    },
    () => false,
  );
}
