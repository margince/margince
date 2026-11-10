// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

const ASCII_LETTER = /^[a-z]$/i;
const ANY_LETTER = /^\p{L}$/u;
const LETTER_CODE = /^Key([A-Z])$/;

/** The key's own letter on a Latin layout. A letter of another script falls
 *  back to its place (`KeyG` is g); AZERTY's "," at `KeyM` is no letter. */
export function pressedLetter(
  event: Pick<KeyboardEvent, "key" | "code">,
): string | null {
  if (ASCII_LETTER.test(event.key)) {
    return event.key.toLowerCase();
  }
  if (!ANY_LETTER.test(event.key)) {
    return null;
  }
  return LETTER_CODE.exec(event.code)?.[1]?.toLowerCase() ?? null;
}

export function isLetterKey(key: string): boolean {
  return ASCII_LETTER.test(key);
}

type ChordPress = Pick<
  KeyboardEvent,
  "key" | "code" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey"
>;

/** ⌘ or Ctrl alone with `key`. Ctrl+Alt is AltGr on Windows, and ⌘⇧B is bold
 *  in an editor, so Alt or Shift refuses. */
export function isChordOf(event: ChordPress, key: string): boolean {
  if (event.metaKey === event.ctrlKey || event.altKey || event.shiftKey) {
    return false;
  }
  return isLetterKey(key) ? pressedLetter(event) === key : event.key === key;
}
