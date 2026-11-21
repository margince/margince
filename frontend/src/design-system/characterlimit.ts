// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ChangeEvent, CompositionEvent } from "react";
import { characterCount } from "../lib/charactercount";

type TextControl = HTMLInputElement | HTMLTextAreaElement;

// The element's own setter, past the one React installs. A value written
// through it still reads as changed, so the input event below reaches onChange.
function writeValue(control: TextControl, value: string) {
  const prototype = Object.getPrototypeOf(control) as object;
  Object.getOwnPropertyDescriptor(prototype, "value")?.set?.call(
    control,
    value,
  );
}

/**
 * cutToCharacterLimit takes out what an edit put past `limit` characters. The
 * edit sits just before the caret, so the cut is there, as the native
 * attribute keeps the start of a paste that does not fit. It returns whether
 * it cut anything.
 */
export function cutToCharacterLimit(
  control: TextControl,
  limit: number,
): boolean {
  const value = control.value;
  const excess = characterCount(value) - limit;
  if (excess <= 0) return false;
  // An email field has no caret to read or set, so its tail is cut.
  const caret = control.selectionEnd;
  const before = caret === null ? [] : [...value.slice(0, caret)];
  if (caret === null || before.length < excess) {
    writeValue(control, [...value].slice(0, limit).join(""));
    return true;
  }
  const kept = before.slice(0, before.length - excess).join("");
  writeValue(control, kept + value.slice(caret));
  control.setSelectionRange(kept.length, kept.length);
  return true;
}

function composing(event: Event): boolean {
  return "isComposing" in event && event.isComposing === true;
}

type LimitedProps<E extends TextControl> = Readonly<{
  maxLength?: number;
  onChange?: (event: ChangeEvent<E>) => void;
  onCompositionEnd?: (event: CompositionEvent<E>) => void;
}>;

/**
 * withCharacterLimit holds `maxLength` in characters, the server's unit, on
 * the control's own handlers. The native attribute counts UTF-16 units, so it
 * is never set. An IME word is cut once it is picked, not while it is typed.
 */
export function withCharacterLimit<E extends TextControl>({
  maxLength,
  onChange,
  onCompositionEnd,
}: LimitedProps<E>) {
  if (maxLength === undefined) return { onChange, onCompositionEnd };
  return {
    "data-character-limit": maxLength,
    onChange: (event: ChangeEvent<E>) => {
      if (!composing(event.nativeEvent)) {
        cutToCharacterLimit(event.currentTarget, maxLength);
      }
      onChange?.(event);
    },
    onCompositionEnd: (event: CompositionEvent<E>) => {
      onCompositionEnd?.(event);
      const control = event.currentTarget;
      if (cutToCharacterLimit(control, maxLength)) {
        control.dispatchEvent(new Event("input", { bubbles: true }));
      }
    },
  };
}
