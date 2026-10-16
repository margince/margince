import { type RefObject, useEffect } from "react";
import { coveredByDialog } from "../design-system/dialogfocus";

/**
 * Dismissal for a popover that owns the document while it is open.
 *
 * The listeners live on the document so Escape works from anywhere inside the
 * popover and any outside click closes it. The click listener is registered a
 * tick late, so the click that OPENED the popover does not immediately close it
 * again.
 *
 * One implementation, every popover in the chrome — the strip's account menu and
 * the theme flyout inside it, the agent dock at the foot of the content column,
 * and the phone sheet: a second copy of this is how two popovers in the same
 * product end up dismissing differently.
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
      // One keystroke closes one layer. A row inside may open a layer of its
      // own (the appearance choice does), and both dismissals listen on the
      // document, so without this a single Escape would collapse the inner
      // layer AND this one — leaving the reader two steps from where they were.
      // The inner layer announces itself through a trigger that has EXPANDED a
      // region it NAMES, and both halves are load-bearing. `aria-expanded`
      // alone also matches the panel's own opener when that opener lives inside
      // it — the phone sheet's "More" is in the bar the sheet grows out of, so
      // a guard reading only the flag stood the sheet down against itself and
      // Escape stopped closing it at all. And what KIND of layer a trigger
      // opens (a menu, a group of radios, a listbox) is the inner surface's
      // business, so reading `aria-haspopup` stood down for one shape only.
      if (
        panel.current?.querySelector('[aria-expanded="true"][aria-controls]')
      ) {
        return;
      }
      dismiss();
    };
    // OUTSIDE clicks only. A listener that fired for every click dismissed the
    // popover when the click was inside it — which, for a popover as large as
    // the phone sheet, meant its own account block closed the sheet out from
    // under the row being pressed. A click that should BOTH act and close (a
    // destination, a settings link) says so at the call site.
    const onClick = (event: globalThis.MouseEvent) => {
      const target = event.target;
      if (target instanceof Node && panel.current?.contains(target)) {
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
