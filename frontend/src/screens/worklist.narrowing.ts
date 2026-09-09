// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which of the day's per-category figures a given narrowing is measured over.
//
// Its own file because it belongs to neither of the two it sits between and both
// are frozen at their length: `worklist.copy.ts` writes the completeness
// sentence and `worklist.header.tsx` owns the lane vocabulary, and this is the
// join — the rule that says a narrowing is not always one category.

import type { Worklist, WorklistFilter } from "./worklist.queries";

/**
 * The per-category figures a narrowing is measured over, or null where they
 * cannot express it.
 *
 * The seven kinds are one category each and `all` is all of them. NEITHER
 * link-only narrowing is a category, and neither can be assembled from these
 * figures:
 *
 *   - `changed_since_brief` cuts across every category on a per-row freshness
 *     the counts do not carry.
 *   - `except_decisions` excludes by SOURCE — the rows a brief draws as cards
 *     are the `approval` ones — and these figures are keyed by category. The
 *     complement of the `decisions` CATEGORY is a near-miss, not the same set:
 *     it drops an introduction request the server keeps. `reach` carries the
 *     per-source figures, but summing it here would be a second copy of the
 *     source-to-category map the contract says the browser must not hold.
 *
 * Null rather than an empty list, and the difference is the defect: an empty
 * list sums to zero considered, which every reader of these figures reads as a
 * complete page. A narrowing that hid fifteen rows reported nothing hidden.
 *
 * A caller must therefore say nothing rather than compute over the answer. The
 * arithmetic would return null too, but by accident — and the accident stops
 * being harmless the moment a caller grows an arm that prints a zero.
 */
export function countsUnder(
  day: Worklist,
  filter: WorklistFilter,
): Worklist["counts"] | null {
  if (filter === "all") {
    return day.counts;
  }
  if (filter === "except_decisions" || filter === "changed_since_brief") {
    return null;
  }
  return day.counts.filter((count) => count.category === filter);
}
