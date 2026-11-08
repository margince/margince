// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { categoryInView, UPCOMING, type WorklistView } from "./worklistviews";

type Worklist = components["schemas"]["Worklist"];
type WorklistFilter = NonNullable<Worklist["filter"]>;
type WorklistCount = Worklist["counts"][number];

// A pill's count when it is exact, else nothing. See pillFigure.
export function pillCount(
  day: Worklist,
  filter: WorklistFilter,
  view?: WorklistView,
): number | undefined {
  const figure = pillFigure(day, filter, view);
  return figure?.floor === false ? figure.count : undefined;
}

/** A count a pill draws, and whether more may stand behind it. */
export type CountFigure = { count: number; floor: boolean };

// What a pill draws: the exact count, or else what was counted, marked as a
// floor. A category is exact when it has the server's `total`, or was read
// whole from sources that all loaded. A floor of none draws nothing, because
// "0" would claim the cut empty and "0+" says nothing.
export function pillFigure(
  day: Worklist,
  filter: WorklistFilter,
  view?: WorklistView,
): CountFigure | undefined {
  const counted =
    filter === "all"
      ? outstandingCounts(day, view)
      : day.counts.filter((count) => count.category === filter);
  if (filter !== "all" && counted.length === 0) return undefined;
  const count = counted.reduce(
    (sum, entry) => sum + (exactCount(entry) ?? entry.considered),
    0,
  );
  const floor =
    day.scope_truncated === true ||
    counted.some((entry) => exactCount(entry) === undefined) ||
    shortOfUnreadRows(day, filter, view);
  if (floor && count === 0) return undefined;
  return { count, floor };
}

// A source that did not load leaves its category short, unless the category
// kept a `total`. A tail source that fails on a later page leaves the total
// the first page stated, and that total still counts the source's rows.
function shortOfUnreadRows(
  day: Worklist,
  filter: WorklistFilter,
  view?: WorklistView,
): boolean {
  const totalled = new Set<string>(
    day.counts
      .filter((count) => count.total !== undefined)
      .map((count) => count.category),
  );
  return unreadEntries(day, filter, view).some(
    (entry) => entry.category === undefined || !totalled.has(entry.category),
  );
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
  return new Set(unreadEntries(day, filter, view).map((entry) => entry.source))
    .size;
}

function unreadEntries(
  day: Worklist,
  filter: WorklistFilter,
  view?: WorklistView,
): Worklist["sources_unavailable"] {
  return day.sources_unavailable
    .filter((entry) => entry.contributes_rows !== false)
    .filter(
      (entry) =>
        entry.category === undefined ||
        (filter === "all"
          ? view === undefined || categoryInView(entry.category, view)
          : entry.category === filter),
    );
}

/**
 * Whether a page says the list it ends is whole: no source failed to answer,
 * the scope was not cut short, and no kind of work holds more than it showed.
 * An absent next cursor alone does not say this.
 */
export function listIsComplete(page: Worklist | undefined): boolean {
  if (!page) return false;
  return (
    page.scope_truncated !== true &&
    (page.sources_unavailable ?? []).length === 0 &&
    !(page.counts ?? []).some((count) => count.more_available)
  );
}
