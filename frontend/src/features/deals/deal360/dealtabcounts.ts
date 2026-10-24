// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import type { RecordTimeline } from "../../../design-system/recordtimeline";
import { useDealLegalReviews } from "../../../lib/legalreviews.queries";
import { useDealDocuments } from "../dealfiles";

type Deal = components["schemas"]["Deal"];
type PageInfo = components["schemas"]["PageInfo"];

export type DealTabCounts = Partial<
  Record<"files" | "legal" | "history", number>
>;

// How many rows a page holds in all. It is the server's total where it counts,
// the rows themselves when nothing is left behind, else no figure (the body
// has no page). A count missing is not a zero.
export function pageCount(
  rows: readonly unknown[] | undefined,
  page: PageInfo | undefined,
): number | undefined {
  if (!rows || !page) {
    return undefined;
  }
  if (page.total !== undefined) {
    return page.total;
  }
  return page.has_more ? undefined : rows.length;
}

// The figure beside a tab label. An empty tab carries none: its label alone
// already says there is nothing behind it.
export function tabFigure(count: number | undefined): number | undefined {
  return count === 0 ? undefined : count;
}

/**
 * The figures beside the deal's tab labels, off the same reads the tabs make,
 * so a label and the body under it never disagree. History is counted only
 * once the thread has no page left behind: the feed states no total.
 */
export function useDealTabCounts(
  deal: Deal,
  legalReview: boolean,
  thread: Pick<RecordTimeline, "activities" | "hasNextPage" | "isSuccess">,
): DealTabCounts {
  const documents = useDealDocuments(deal.id);
  const reviews = useDealLegalReviews(
    deal.id,
    legalReview && !deal.archived_at,
  );
  return {
    files: tabFigure(pageCount(documents.data?.data, documents.data?.page)),
    legal: tabFigure(reviews.data?.length),
    history:
      thread.isSuccess && !thread.hasNextPage
        ? tabFigure(thread.activities.length)
        : undefined,
  };
}
