// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The three work views and the selector between them. The server splits the
// day by view (`view` on GET /worklist); this draws the choice and its counts.

import { SegmentedControl } from "../../design-system/atoms";
import { floorFigure } from "../../format/figure";
import { formatNumber } from "../../format/format";
import { type Locale, translatePlural, useT } from "../../i18n";
import { outstandingCounts, pillFigure } from "../../lib/worklistcounts";
import { WORKLIST_VIEWS, type WorklistView } from "../../lib/worklistviews";
import type { Worklist, WorklistFilter } from "./worklist.queries";

/**
 * The category buttons each view draws. Decisions and System each hold one
 * category, so a button row there would repeat the view's own name.
 */
export const VIEW_PILLS = {
  today: [
    "all",
    "customer_waiting",
    "leads",
    "deals_at_risk",
    "meetings",
    "tasks",
    "relationships",
  ],
  decisions: [],
  system: [],
} as const satisfies Record<WorklistView, readonly WorklistFilter[]>;

/** A filter some view draws as a button. */
export type ViewPill = (typeof VIEW_PILLS)["today"][number];

/** The view's own count, exact or a floor; never a count of another view. */
export function viewCount(day: Worklist, view: WorklistView) {
  return pillFigure(day, "all", view);
}

/** The headline of one view: how much it holds, in its own unit. */
export function viewHeadline(
  day: Worklist,
  view: WorklistView,
  locale: Locale,
): string {
  // A view no category was counted for has only the summary to go on. The
  // figure is the tab's own, so the heading and the tab never disagree.
  const figure = outstandingCounts(day, view).length
    ? (viewCount(day, view) ?? { count: day.summary.total, floor: true })
    : { count: day.summary.total, floor: false };
  return translatePlural(locale, `worklist.view.total.${view}`, figure.count, {
    count: floorFigure(
      formatNumber(figure.count, locale),
      figure.floor,
      figure.count,
    ),
  });
}

/** Today, Decisions and System, each with its count, or the floor it counted. */
export function WorklistViewPicker({
  day,
  view,
  onView,
}: Readonly<{
  day: Worklist;
  view: WorklistView;
  onView: (next: WorklistView) => void;
}>) {
  const t = useT();
  const counts: Partial<Record<WorklistView, number>> = {};
  const floors: Partial<Record<WorklistView, boolean>> = {};
  for (const option of WORKLIST_VIEWS) {
    const figure = viewCount(day, option);
    if (figure === undefined) continue;
    counts[option] = figure.count;
    floors[option] = figure.floor;
  }
  return (
    <SegmentedControl
      options={WORKLIST_VIEWS}
      value={view}
      onChange={onView}
      label={t("worklist.view.label")}
      labels={{
        today: t("worklist.view.today"),
        decisions: t("worklist.view.decisions"),
        system: t("worklist.view.system"),
      }}
      counts={counts}
      floors={floors}
    />
  );
}
