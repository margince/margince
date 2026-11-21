// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { graphemeSegmenter } from "../format/formatters";

/**
 * characterCount is how many characters the server counts in `text`: code
 * points, as Postgres `char_length` and Go's rune count do. `String.length`
 * counts UTF-16 units, so an emoji would count twice.
 */
export function characterCount(text: string): number {
  return [...text].length;
}

/**
 * graphemes splits `text` into what a reader sees as one character, so a cut
 * never splits a flag, a joined emoji or an accent. Code points stand in where
 * the browser has no Intl.Segmenter.
 */
export function graphemes(text: string): string[] {
  if (typeof Intl.Segmenter !== "function") {
    return Array.from(text);
  }
  return Array.from(graphemeSegmenter().segment(text), (part) => part.segment);
}
