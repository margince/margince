// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * What a stylesheet DECLARES, for the gates that read one.
 *
 * Every CSS gate in this tree opens with the same problem: the property it
 * hunts for appears in the paragraph explaining the rule at least as often as
 * in the rule, and a commented-out declaration declares nothing. Three of them
 * were each carrying their own copy of the answer.
 */

/**
 * The same text with every comment blanked to spaces of its own length.
 *
 * Blanked rather than deleted, so every offset into the result still points at
 * the same character of the original: a gate that reads an in-line waiver finds
 * it in the comment beside the declaration, which it can only do while the two
 * texts still line up.
 */
export function withoutComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    " ".repeat(comment.length),
  );
}
