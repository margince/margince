// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useState } from "react";
import { liveDialogs } from "../design-system/dialogfocus";
import { screenEnabled, useDisabledCoreFeatures } from "../lib/corefeatures";
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
  const disabled = useDisabledCoreFeatures();
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
    if (
      route.id === undefined &&
      CREATE_SCREENS.has(route.screen) &&
      screenEnabled(route.screen, disabled)
    ) {
      navigateReplacing(
        { screen: route.screen, id: CREATE_ID },
        currentParams(),
      );
    } else {
      show("create");
    }
  });
}

/** The palette's state with its keys bound. `open` is null until first asked,
 *  so the palette's code loads only then. */
export function useCommandPalette(route: Route) {
  const [open, setOpen] = useState<boolean | null>(null);
  const [mode, setMode] = useState<PaletteMode>("all");
  const show = useCallback((next: PaletteMode) => {
    setMode(next);
    setOpen(true);
  }, []);
  const hide = useCallback(() => setOpen(false), []);
  usePaletteShortcuts(open === true, show, hide, route);
  return { open, mode, show, hide };
}
