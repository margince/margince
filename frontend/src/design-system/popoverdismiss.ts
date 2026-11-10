// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect } from "react";
import { coveredByDialog, panelsOpenedFrom } from "./dialogfocus";

/**
 * Escape, or a press outside the panel and its anchor, closes the panel.
 * A dialog raised over it, or a list opened from inside it, owns the key or
 * the press instead.
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
      if (
        event.key !== "Escape" ||
        coveredByDialog(anchor.current) ||
        panelsOpenedFrom(panel.current).length > 0
      ) {
        return;
      }
      event.preventDefault();
      close("escape");
    };
    const onPointer = (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) {
        return;
      }
      // A dialog raised over the panel, such as a date field's calendar,
      // owns the press as it owns Escape.
      if (coveredByDialog(anchor.current)) {
        return;
      }
      // A select's list is portalled to the body, so a press on its option
      // lands outside the panel; closing then would lose the pick.
      if (
        panelsOpenedFrom(panel.current).some((opened) =>
          opened.contains(target),
        )
      ) {
        return;
      }
      if (
        !anchor.current?.contains(target) &&
        !panel.current?.contains(target)
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
