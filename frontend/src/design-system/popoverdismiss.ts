// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect } from "react";
import { coveredByDialog } from "./dialogfocus";

/**
 * Escape, or a press outside the panel and its anchor, closes the panel,
 * unless a dialog raised over it owns the key or the press.
 *
 * Shared by `Popover` and `usePeek`, which close the same way.
 */
export function usePanelDismiss({
  open,
  anchor,
  panel,
  close,
}: Readonly<{
  open: boolean;
  anchor: RefObject<HTMLElement | null>;
  panel: RefObject<HTMLElement | null>;
  // Says which way it was put away. After Escape focus goes back to the
  // anchor. An anchor that opens on focus must know that was not a reader
  // asking to see the panel again.
  close: (how: "escape" | "outside") => void;
}>): void {
  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || coveredByDialog(anchor.current)) {
        return;
      }
      event.preventDefault();
      close("escape");
    };
    const onPointer = (event: MouseEvent) => {
      if (!(event.target instanceof Node)) {
        return;
      }
      // A dialog raised over the panel, such as a date field's calendar,
      // owns the press as it owns Escape.
      if (coveredByDialog(anchor.current)) {
        return;
      }
      if (
        !anchor.current?.contains(event.target) &&
        !panel.current?.contains(event.target)
      ) {
        close("outside");
      }
    };
    globalThis.addEventListener("keydown", onKey);
    globalThis.addEventListener("mousedown", onPointer);
    return () => {
      globalThis.removeEventListener("keydown", onKey);
      globalThis.removeEventListener("mousedown", onPointer);
    };
  }, [open, anchor, panel, close]);
}
