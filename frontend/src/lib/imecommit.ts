// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Whether a key belongs to an input method rather than to the field. An Enter
 * that picks a Telex or kana candidate is not a submit. Safari ends the
 * composition before keydown and reports the key only as keyCode 229.
 */
export function isImeCommit(event: { readonly nativeEvent: KeyboardEvent }) {
  return event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229;
}
