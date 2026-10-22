// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lines the distilling panel reads back from a source: whole sentences of
// the reader's own prose, picked deterministically so the same file always
// shows the same lines. Nothing here counts toward anything — every number on
// the voice step is the server's — this only chooses what to quote.

// A sentence shorter than this is a fragment ("Thanks.", "Best, Anna"); one
// longer wraps to a paragraph in a panel meant to be read at a glance.
const MIN_WORDS = 6;
const MAX_WORDS = 28;
export const EXCERPT_LINES_PER_SOURCE = 8;

/**
 * Up to `EXCERPT_LINES_PER_SOURCE` sentences from `content`, in the order they
 * were written, each between six and twenty-eight words. Splits on sentence
 * punctuation and line breaks; a text with no sentence of that length yields
 * nothing, which the panel renders as nothing rather than as a fragment.
 */
export function excerptLines(content: string): string[] {
  const out: string[] = [];
  for (const raw of content.split(/(?<=[.!?])\s+|\n+/)) {
    const line = raw.replace(/\s+/g, " ").trim();
    const words = line === "" ? 0 : line.split(" ").length;
    if (words >= MIN_WORDS && words <= MAX_WORDS) {
      out.push(line);
      if (out.length === EXCERPT_LINES_PER_SOURCE) {
        break;
      }
    }
  }
  return out;
}

/**
 * The words a line is lit by: its two longest, as their positions in the
 * line's word list. Length is a stand-in for salience that costs nothing and
 * never claims to be a measurement — the panel marks them as the eye's
 * anchors, not as findings.
 */
export function emphasisIndices(words: readonly string[]): ReadonlySet<number> {
  const ranked = words
    .map((word, index) => ({ index, length: word.replace(/\W/g, "").length }))
    .filter((entry) => entry.length >= 6)
    .sort((a, b) => b.length - a.length || a.index - b.index)
    .slice(0, 2)
    .map((entry) => entry.index);
  return new Set(ranked);
}
