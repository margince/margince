// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * incompleteGraph says the connection graph a page read is not the whole one:
 * it capped its contact ring, or it withheld groups the caller may not read.
 * Either way the routes below it are a subset, and both the empty answer and
 * the found-someone answer have to say so.
 */
export function incompleteGraph(graph: {
  groups_omitted?: unknown[];
  dropped_count?: number;
}): boolean {
  return (
    (graph.groups_omitted?.length ?? 0) > 0 || (graph.dropped_count ?? 0) > 0
  );
}
