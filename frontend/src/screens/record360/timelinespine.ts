// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A record's thread, read off the timeline rows the page already holds.
//
// A deal and a lead have no composite read the way an account and a contact
// do: their timeline arrives as a page of activities. The spine wants one
// source, so this assembles it — from the same rows the timeline tab draws, so
// the thread cannot name a conversation the tab does not have — and adds
// nothing but the two directional dates the rows imply.

import type { components } from "../../api/schema";
import type { SpineSource } from "./spine";

type Activity = components["schemas"]["Activity"];

/**
 * timelineSpineSource reads the thread's source out of a timeline page.
 *
 * `asOf` is the instant the reading is made at — the page has no server
 * `as_of` for these records, so the caller passes its own clock once and every
 * "how long ago" on the thread is measured from the same moment.
 *
 * The directional dates are the newest inbound and outbound rows. A note or
 * a hand-logged call carries no direction and is neither: what was said TO
 * the buyer and what came back are the two facts that say whose move it is,
 * and a note we wrote ourselves is not a message they owe an answer to.
 *
 * `hasMore` says the page was cut, so the thread counts what it did not draw
 * as "more" rather than as a number a reader could check and find wrong.
 */
export function timelineSpineSource(
  activities: readonly Activity[],
  asOf: string,
  hasMore: boolean,
): SpineSource {
  const at = Date.parse(asOf);
  return {
    as_of: asOf,
    last_inbound_at: newest(activities, "inbound"),
    last_outbound_at: newest(activities, "outbound"),
    activities: { data: activities, page: { has_more: hasMore } },
    next_steps: {
      data: activities
        .filter((row) => row.kind === "task" && !row.is_done)
        .map((task) => ({
          activity_id: task.id,
          subject: task.subject ?? "",
          due_at: task.due_at,
          overdue: Boolean(task.due_at && Date.parse(task.due_at) < at),
        })),
    },
  };
}

// The newest row that went the given way, or nothing. Compared as instants
// rather than as strings, because the same moment is written with any offset.
function newest(
  activities: readonly Activity[],
  direction: "inbound" | "outbound",
): string | undefined {
  let latest: string | undefined;
  for (const row of activities) {
    if (row.direction !== direction) {
      continue;
    }
    if (!latest || Date.parse(row.occurred_at) > Date.parse(latest)) {
      latest = row.occurred_at;
    }
  }
  return latest;
}
