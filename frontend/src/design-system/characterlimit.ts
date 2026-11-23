// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ChangeEvent, CompositionEvent, FocusEvent } from "react";
import { characterCount, graphemes } from "../lib/charactercount";

type TextControl = HTMLInputElement | HTMLTextAreaElement;

// The value each control held before the edit now being read. A stored value
// over the limit stays whole: only what an edit added is ever cut.
const before = new WeakMap<TextControl, string>();

// The start of the grapheme that holds `index`, so a cut never splits an emoji
// sequence or a letter from its combining mark.
function graphemeStart(value: string, index: number): number {
  let start = 0;
  for (const grapheme of graphemes(value)) {
    if (start + grapheme.length > index) break;
    start += grapheme.length;
  }
  return start;
}

type Cut = Readonly<{ value: string; caret: number | null }>;

// The value with what the last edit put past `limit` taken out, or undefined
// when nothing goes. The edit ends at the caret, so the cut keeps its start.
// The native attribute also keeps the start of a paste that does not fit.
function cutOf(control: TextControl, limit: number): Cut | undefined {
  const value = control.value;
  const previous = before.get(control);
  const excess = characterCount(value) - limit;
  if (excess <= 0) return undefined;
  // An email field has no caret, so its edit is taken to end the value.
  const caret = control.selectionEnd ?? value.length;
  // Only what the edit added may go, so a deletion cuts nothing.
  const added = previous === undefined ? caret : value.length - previous.length;
  if (added <= 0) return undefined;
  const start = Math.max(0, caret - added);
  const points = [...value.slice(start, caret)];
  const kept = points.slice(0, Math.max(0, points.length - excess)).join("");
  const at = Math.max(start, graphemeStart(value, start + kept.length));
  return {
    value: value.slice(0, at) + value.slice(caret),
    caret: control.selectionEnd === null ? null : at,
  };
}

function place(control: TextControl, caret: number | null) {
  if (caret !== null) control.setSelectionRange(caret, caret);
}

function composing(event: Event): boolean {
  return "isComposing" in event && event.isComposing === true;
}

type LimitedProps<E extends TextControl> = Readonly<{
  maxLength?: number;
  onChange?: (event: ChangeEvent<E>) => void;
  onCompositionEnd?: (event: CompositionEvent<E>) => void;
  onFocus?: (event: FocusEvent<E>) => void;
}>;

/**
 * withCharacterLimit holds `maxLength` in characters, the server's unit, on
 * the control's own handlers. The native attribute counts UTF-16 units, so it
 * is never set. Like it, the limit refuses what an edit adds and never takes
 * away stored text. An IME word is cut once it is picked, not while typed.
 */
export function withCharacterLimit<E extends TextControl>({
  maxLength,
  onChange,
  onCompositionEnd,
  onFocus,
}: LimitedProps<E>) {
  if (maxLength === undefined) return { onChange, onCompositionEnd, onFocus };
  return {
    "data-character-limit": maxLength,
    onFocus: (event: FocusEvent<E>) => {
      before.set(event.currentTarget, event.currentTarget.value);
      onFocus?.(event);
    },
    onChange: (event: ChangeEvent<E>) => {
      const control = event.currentTarget;
      if (!composing(event.nativeEvent)) {
        const cut = cutOf(control, maxLength);
        // Through React's own setter, so its record of the value agrees with
        // the screen and the next key past the limit is cut too.
        if (cut) {
          control.value = cut.value;
          place(control, cut.caret);
        }
        before.set(control, control.value);
      }
      onChange?.(event);
    },
    onCompositionEnd: (event: CompositionEvent<E>) => {
      onCompositionEnd?.(event);
      const control = event.currentTarget;
      const cut = cutOf(control, maxLength);
      if (!cut) return;
      // Past React's setter, so the input event below reads as a change and
      // reaches onChange with the cut value.
      const prototype = Object.getPrototypeOf(control) as object;
      Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(
        control,
        cut.value,
      );
      place(control, cut.caret);
      before.set(control, cut.value);
      control.dispatchEvent(new Event("input", { bubbles: true }));
    },
  };
}
