// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Worklist = components["schemas"]["Worklist"];
type WorklistFilter = NonNullable<Worklist["filter"]>;

// A pill draws a count only when it is exact. A category read to its bound has
// a floor, and `FilterPills` draws no number for an absent count.
export function pillCount(
  day: Worklist,
  filter: WorklistFilter,
): number | undefined {
  if (day.scope_truncated || unreadSources(day, filter) > 0) return undefined;
  if (filter === "all") {
    return day.counts.some((count) => count.more_available)
      ? undefined
      : day.counts.reduce((total, count) => total + count.considered, 0);
  }
  const found = day.counts.find((count) => count.category === filter);
  if (!found || found.more_available) {
    return undefined;
  }
  return found.considered;
}

// A source that never answered may hold rows of this cut, so the line must not
// fall silent over it. A side read that puts no row on the queue cannot.
export function unreadSources(day: Worklist, filter: WorklistFilter): number {
  const sources = new Set(
    day.sources_unavailable
      .filter((entry) => entry.contributes_rows !== false)
      .filter(
        (entry) =>
          filter === "all" ||
          entry.category === undefined ||
          entry.category === filter,
      )
      .map((entry) => entry.source),
  );
  return sources.size;
}
