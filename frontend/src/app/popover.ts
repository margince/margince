import { type RefObject, useEffect } from "react";
import {
  coveredByDialog,
  panelsOpenedFrom,
} from "../design-system/dialogfocus";

/**
 * Dismissal for a popover that owns the document while it is open: Escape from
 * anywhere, or a click outside. The click listener is registered a tick late, so
 * the click that opened the popover does not close it again. Every popover in
 * the chrome uses this one, so they all dismiss the same way.
 */
export function usePopoverDismiss(
  open: boolean,
  panel: RefObject<HTMLElement | null>,
  dismiss: () => void,
): void {
  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape" || coveredByDialog(panel.current)) {
        return;
      }
      // One keystroke closes one layer. An inner layer (the appearance choice)
      // shows itself as a trigger with `aria-expanded` and `aria-controls`.
      // Both are needed: the flag alone also matches the panel's own opener
      // when it lives inside, and Escape would never close that panel.
      if (
        panel.current?.querySelector('[aria-expanded="true"][aria-controls]')
      ) {
        return;
      }
      dismiss();
    };
    // Outside clicks only: a press inside the popover acts on it. A click that
    // should both act and close (a destination) says so at the call site.
    const onClick = (event: globalThis.MouseEvent) => {
      const within = panel.current;
      // The path, not `contains`: a press that replaces its own button leaves
      // a target the panel no longer holds, and that press was still inside.
      const path = event.composedPath();
      if (within !== null && path.includes(within)) {
        return;
      }
      // A dialog raised from the panel owns the press, and so does a menu the
      // panel opened: both are portalled to the body, outside it.
      if (
        coveredByDialog(within) ||
        panelsOpenedFrom(within).some((opened) => path.includes(opened))
      ) {
        return;
      }
      dismiss();
    };
    document.addEventListener("keydown", onKey);
    const timer = window.setTimeout(
      () => document.addEventListener("click", onClick),
      0,
    );
    return () => {
      document.removeEventListener("keydown", onKey);
      window.clearTimeout(timer);
      document.removeEventListener("click", onClick);
    };
  }, [open, panel, dismiss]);
}
