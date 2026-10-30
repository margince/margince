// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The three work views of the Worklist, as the server splits them. This is a
// declared mirror of `viewOfFilter` and `inView` in
// backend/internal/compose/attention/views.go; the server applies the split.

import type { components } from "../api/schema";

type Worklist = components["schemas"]["Worklist"];
type WorklistFilter = NonNullable<Worklist["filter"]>;
type WorklistCategory = Worklist["counts"][number]["category"];

export type WorklistView = NonNullable<Worklist["view"]>;

/** The views in the order the selector draws them; Today is the default. */
export const WORKLIST_VIEWS = [
  "today",
  "decisions",
  "system",
] as const satisfies readonly WorklistView[];

export const DEFAULT_VIEW: WorklistView = "today";

/** The category that holds work due after today, a section of Today. */
export const UPCOMING = "upcoming" satisfies WorklistCategory;

/** The view a filter value belongs to; `all` belongs to every view. */
export function viewOfFilter(filter: WorklistFilter): WorklistView | undefined {
  if (filter === "all") return undefined;
  if (filter === "decisions" || filter === "system") return filter;
  return "today";
}

/** Whether a category's work belongs to a view. A string, because a missing source names its category untyped. */
export function categoryInView(category: string, view: WorklistView): boolean {
  if (view === "today")
    return category !== "decisions" && category !== "system";
  return category === view;
}

/** The view an address names, reading an old `filter=decisions` link as its view. */
export function viewFromAddress(
  asked: string | undefined,
  filter: WorklistFilter,
): WorklistView {
  const named = WORKLIST_VIEWS.find((view) => view === asked);
  if (named) return named;
  const home = viewOfFilter(filter);
  return home === "decisions" || home === "system" ? home : DEFAULT_VIEW;
}
