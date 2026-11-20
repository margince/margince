// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { type FieldLabel, fieldCheckSentence } from "./faultsentence";

/**
 * What is wrong with a whole number typed in a box, in the server's words for
 * the same refusal. Nothing when it is a whole number from `min` to `max`.
 * Spaces around it are allowed. A blank box is the caller's to judge.
 */
export function wholeNumberFault(
  typed: string,
  locale: Locale,
  label: FieldLabel,
  bounds: Readonly<{ min: number; max: number }>,
): string | undefined {
  const trimmed = typed.trim();
  if (!/^-?\d+$/.test(trimmed)) {
    return fieldCheckSentence("not_integer", locale, label);
  }
  const value = Number(trimmed);
  return value < bounds.min || value > bounds.max
    ? fieldCheckSentence("out_of_range", locale, label, {
        min: String(bounds.min),
        max: String(bounds.max),
      })
    : undefined;
}
