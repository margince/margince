// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect } from "react";
import { coveredByDialog } from "./dialogfocus";

/**
 * How an open portalled panel is put away. Escape does it, unless a dialog
 * raised over it owns that key. So does a press anywhere outside both the
 * panel and what opened it. The panel lives at the body. So "outside" has to mean outside
 * both boxes, or a click on a link inside it reads as a click away.
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
