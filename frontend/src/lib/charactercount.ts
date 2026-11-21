// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * characterCount is how many characters the server counts in `text`: code
 * points, as Postgres `char_length` and Go's rune count do. `String.length`
 * counts UTF-16 units, so an emoji would count twice.
 */
export function characterCount(text: string): number {
  return [...text].length;
}
