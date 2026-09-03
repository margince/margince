// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Tag } from "./tagadmin.queries";

/**
 * The words a proposed name is close enough to be a duplicate of.
 *
 * A vocabulary drifts one near-miss at a time: "K5 Conference" beside "K5
 * Conference 2026" beside "k5-conference", each coined by somebody who looked
 * for the word they wanted, did not see it, and added it. The server refuses
 * only an EXACT collision, which catches none of those.
 *
 * So this warns rather than refuses. An admin coining "EV" beside "EV
 * programme" may mean exactly that, and a rule that stopped them would be this
 * tier deciding what the vocabulary is allowed to contain.
 */
export function nearMatches(
  proposed: string,
  vocabulary: readonly Tag[],
): readonly Tag[] {
  const needle = normalizeForCompare(proposed);
  if (needle === "") {
    return [];
  }
  return vocabulary.filter((tag) => {
    const other = normalizeForCompare(tag.name);
    if (other === "") {
      return false;
    }
    // Both directions: "EV" typed against an existing "EV programme" and "EV
    // programme" typed against an existing "EV" are the same near-duplicate
    // seen from its two ends.
    return (
      other === needle ||
      containsWhole(other, needle) ||
      containsWhole(needle, other)
    );
  });
}

/**
 * Whether `haystack` contains `needle` as WHOLE WORDS, not as a substring.
 *
 * A plain `includes` makes a short word match everything: a vocabulary holding
 * a tag named "a" warns on every proposed name containing that letter, and a
 * warning that fires on everything is one an admin learns to click past — which
 * costs the real near-duplicates it exists to catch.
 */
function containsWhole(haystack: string, needle: string): boolean {
  const words = haystack.split(" ");
  const sought = needle.split(" ");
  return words.some((_, at) =>
    sought.every((word, offset) => words[at + offset] === word),
  );
}

/**
 * A name reduced to what a reader would call "the same word".
 *
 * Case, surrounding space, inner runs of space, and the separators a person
 * reaches for when a space feels wrong — a hyphen and an underscore. Not
 * punctuation in general: "K5" and "K5!" are the same word, but stripping
 * everything would make "C++" and "C" one, which is a distinction somebody
 * meant.
 */
function normalizeForCompare(value: string): string {
  return value.trim().toLowerCase().replace(/[-_]+/g, " ").replace(/\s+/g, " ");
}
