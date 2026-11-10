// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { liveDialogs } from "../design-system/dialogfocus";
import { navigate, type Screen } from "../lib/router";
import { useShortcutHandler } from "../lib/shortcuts";
import { CREATE_ID, CREATE_SCREENS } from "./nav";

/** What the palette lists: every command, or only the ones that create. */
export type PaletteMode = "all" | "create";

/**
 * The palette's bindings: ⌘K toggles it, `/` opens it, and `C` opens the
 * screen's own create form or, where it has none, the palette's create list.
 */
export function usePaletteShortcuts(
  open: boolean,
  show: (mode: PaletteMode) => void,
  hide: () => void,
  screen: Screen,
): void {
  useShortcutHandler("search", () => {
    // An open dialog makes the app unreachable; only the palette's own layer may close it.
    if (open) {
      hide();
    } else if (liveDialogs().length === 0) {
      show("all");
    }
  });
  useShortcutHandler("search.slash", () => show("all"));
  useShortcutHandler("create", () => {
    if (CREATE_SCREENS.has(screen)) {
      navigate({ screen, id: CREATE_ID });
    } else {
      show("create");
    }
  });
}
