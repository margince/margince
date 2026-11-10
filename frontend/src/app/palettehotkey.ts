import { useEffect } from "react";
import { liveDialogs } from "../design-system/dialogfocus";
import { modifierCap, modifierKeyName } from "./hotkeys";

const PALETTE_KEY = "k";

/**
 * The ⌘K / Ctrl K chord as one cap per key. Both modifiers work, but a surface
 * shows only the one the reader's keyboard has. Pure in `platform`, so a test
 * needs no stub.
 */
export function paletteHotkeyCaps(platform: string): readonly string[] {
  return [modifierCap(platform), PALETTE_KEY.toUpperCase()];
}

/** The chord the caps print, as `aria-keyshortcuts` takes it. */
export function paletteKeyShortcuts(platform: string): string {
  return `${modifierKeyName(platform)}+${PALETTE_KEY.toUpperCase()}`;
}

export function usePaletteHotkey(
  open: boolean,
  setOpen: (open: boolean) => void,
) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLowerCase() === PALETTE_KEY
      ) {
        event.preventDefault();
        // An open dialog makes the app unreachable. Only an already-open
        // palette may handle the shortcut while a dialog exists.
        if (open || liveDialogs().length === 0) {
          setOpen(!open);
        }
      }
    };
    globalThis.addEventListener("keydown", onKey);
    return () => globalThis.removeEventListener("keydown", onKey);
  }, [open, setOpen]);
}
