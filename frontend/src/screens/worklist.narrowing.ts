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
 * Here rather than beside `completenessText`, because the answer depends on the
 * vocabulary this file owns. The seven kinds are one category each and `all` is
 * all of them. The two link-only narrowings are neither: `except_decisions` is
 * every category but one, which these figures DO express, and
 * `changed_since_brief` cuts across them per row on a freshness the counts do
 * not carry — so it has no answer here and says so.
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
  if (filter === "except_decisions") {
    return day.counts.filter((count) => count.category !== "decisions");
  }
  if (filter === "changed_since_brief") {
    return null;
  }
  return day.counts.filter((count) => count.category === filter);
}

// The ways a row can be put down, derived from the item rather than spelled
// again: the contract declares them inline, and a hand-written union would go
// stale the moment the server gained a fourth — silently, because nothing
// compares the two.
