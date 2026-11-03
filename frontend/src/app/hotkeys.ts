// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The sidebar's chord and the platform's modifier cap. The sidebar's handler,
// the palette's caps and the shortcuts dialog read them.

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
