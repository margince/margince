// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MouseEvent } from "react";

// A modified click asks the browser for a new tab, a window or a saved copy.
// A link's own handler leaves that click alone.
export function opensElsewhere(event: MouseEvent<HTMLAnchorElement>): boolean {
  return event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
}
