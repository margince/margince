// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { liveDialogs } from "../design-system/dialogfocus";
import { navigateReplacing, type Route } from "../lib/router";
import { useShortcutHandler } from "../lib/shortcuts";
import { currentParams } from "../lib/urlstate";
import { CREATE_ID, CREATE_SCREENS } from "./nav";

/** What the palette lists: every command, or only the ones that create. */
export type PaletteMode = "all" | "create";

/**
 * The palette's bindings: ⌘K toggles it, `/` opens it, and `C` opens a list's
 * own create form on the list itself, and the palette's create list elsewhere.
 */
export function usePaletteShortcuts(
  open: boolean,
  show: (mode: PaletteMode) => void,
  hide: () => void,
  route: Route,
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
    // Only on the list: replacing a record's address would erase it from history.
    if (route.id === undefined && CREATE_SCREENS.has(route.screen)) {
      navigateReplacing(
        { screen: route.screen, id: CREATE_ID },
        currentParams(),
      );
    } else {
      show("create");
    }
  });
}
