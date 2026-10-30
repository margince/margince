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

// One row per contract value, so tsc refuses a value with no view.
// views_mirror_test.go compares both tables with views.go.
const FILTER_VIEW: Readonly<Record<WorklistFilter, WorklistView | null>> = {
  all: null,
  changed_since_brief: "today",
  customer_waiting: "today",
  deals_at_risk: "today",
  decisions: "decisions",
  except_decisions: "today",
  leads: "today",
  meetings: "today",
  relationships: "today",
  system: "system",
  tasks: "today",
  upcoming: "today",
  urgent: "today",
};

const CATEGORY_VIEW: Readonly<Record<WorklistCategory, WorklistView>> = {
  customer_waiting: "today",
  deals_at_risk: "today",
  decisions: "decisions",
  leads: "today",
  meetings: "today",
  relationships: "today",
  system: "system",
  tasks: "today",
  upcoming: "today",
};

const categoryViews = new Map<string, WorklistView>(
  Object.entries(CATEGORY_VIEW),
);

/** The view a filter value belongs to; `all` belongs to every view. */
export function viewOfFilter(filter: WorklistFilter): WorklistView | undefined {
  return FILTER_VIEW[filter] ?? undefined;
}

/** Whether a category's work belongs to a view. A string, because a missing source names its category untyped. */
export function categoryInView(category: string, view: WorklistView): boolean {
  return (categoryViews.get(category) ?? DEFAULT_VIEW) === view;
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
