// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The app-wide chords, each defined once, so the handler that binds a chord and
// the shortcuts dialog that lists it cannot come to disagree.

const APPLE_PLATFORM = /mac|iphone|ipad|ipod/i;

/** The platform modifier as the cap its keyboard prints. */
export function modifierCap(platform: string): string {
  return APPLE_PLATFORM.test(platform) ? "⌘" : "Ctrl";
}

const SIDEBAR_KEY = "b";

type Chord = Pick<
  KeyboardEvent,
  "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey"
>;

/**
 * ⌘B / Ctrl B with one platform modifier and no other. Cmd+Shift+B is bold with
 * a selection in most editors, and a looser match would swallow it.
 */
export function isSidebarChord(event: Chord): boolean {
  return (
    event.metaKey !== event.ctrlKey &&
    !event.altKey &&
    !event.shiftKey &&
    event.key.toLowerCase() === SIDEBAR_KEY
  );
}

export function sidebarHotkeyCaps(platform: string): readonly string[] {
  return [modifierCap(platform), SIDEBAR_KEY.toUpperCase()];
}

/** The key every popover and dialog in the product closes on. */
export const DISMISS_KEY = "Escape";
