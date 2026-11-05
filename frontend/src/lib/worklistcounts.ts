// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { categoryInView, UPCOMING, type WorklistView } from "./worklistviews";

type Worklist = components["schemas"]["Worklist"];
type WorklistFilter = NonNullable<Worklist["filter"]>;
type WorklistCount = Worklist["counts"][number];

// A pill draws a count only when it is exact: the server's `total`, or what it
// considered when no source behind the category was cut. A category read to its
// bound with no total has a floor, and `FilterPills` draws no number for an
// absent count.
export function pillCount(
  day: Worklist,
  filter: WorklistFilter,
  view?: WorklistView,
): number | undefined {
  if (day.scope_truncated || unreadSources(day, filter, view) > 0)
    return undefined;
  if (filter === "all") {
    let sum = 0;
    for (const count of outstandingCounts(day, view)) {
      const exact = exactCount(count);
      if (exact === undefined) return undefined;
      sum += exact;
    }
    return sum;
  }
  const found = day.counts.find((count) => count.category === filter);
  return found === undefined ? undefined : exactCount(found);
}

// The one figure a category may state as exact, or none.
export function exactCount(count: WorklistCount): number | undefined {
  if (count.total !== undefined) return count.total;
  return count.more_available ? undefined : count.considered;
}

// The categories `all` adds up inside a view. Upcoming work is never due now,
// so it never joins the outstanding total.
export function outstandingCounts(
  day: Worklist,
  view?: WorklistView,
): Worklist["counts"] {
  return day.counts.filter(
    (count) =>
      count.category !== UPCOMING &&
      (view === undefined || categoryInView(count.category, view)),
  );
}

// A source that never answered may hold rows of this cut, so the line must not
// fall silent over it. A side read that puts no row on the queue cannot.
export function unreadSources(
  day: Worklist,
  filter: WorklistFilter,
  view?: WorklistView,
): number {
  const sources = new Set(
    day.sources_unavailable
      .filter((entry) => entry.contributes_rows !== false)
      .filter(
        (entry) =>
          entry.category === undefined ||
          (filter === "all"
            ? view === undefined || categoryInView(entry.category, view)
            : entry.category === filter),
      )
      .map((entry) => entry.source),
  );
  return sources.size;
}
