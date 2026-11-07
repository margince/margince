// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MouseEvent } from "react";

// A modified click asks the browser for a new tab, a window or a saved copy.
// A link's own handler leaves that click alone.
export function opensElsewhere(
  event: Pick<
    MouseEvent<HTMLAnchorElement>,
    "metaKey" | "ctrlKey" | "shiftKey" | "altKey"
  >,
): boolean {
  return event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
}

// React bubbles a portal's click into the row that rendered it, so a target
// outside the row is a popover's. A drag that selects text is not a press.
export function opensRow(event: MouseEvent<HTMLElement>): boolean {
  const target = event.target;
  if (!(target instanceof Element) || !event.currentTarget.contains(target)) {
    return false;
  }
  const control = target.closest(
    "a, button, input, select, textarea, label, summary, [role='button'], [role='switch'], [role='menuitem'], [role='checkbox'], [role='tab']",
  );
  if (control !== null && event.currentTarget.contains(control)) {
    return false;
  }
  return globalThis.getSelection?.()?.isCollapsed !== false;
}
