// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";

export const MIN_MINUTES = 15;
export const MAX_MINUTES = 480;
export const MAX_OFFERED = 16;

export function parseLengths(value: string): number[] {
  return value
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean)
    .map(Number);
}

// The one problem a list of extra lengths has, named so the field says which.
export function lengthsProblem(lengths: number[]): MessageKey | undefined {
  if (
    !lengths.every(
      (length) =>
        Number.isInteger(length) &&
        length >= MIN_MINUTES &&
        length <= MAX_MINUTES,
    )
  ) {
    return "scheduling.lengthInvalid";
  }
  if (new Set(lengths).size !== lengths.length) {
    return "scheduling.lengthRepeated";
  }
  return lengths.length > MAX_OFFERED ? "scheduling.lengthsTooMany" : undefined;
}
