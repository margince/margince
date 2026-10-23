// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Whose move it is is decided by the server, not here. `DealStatusCard.reply_to`
// is the inbound message nobody has answered, picked by the rule that also
// decides whether the email box offers a reply. And it decides one direction
// only. Its absence says the buyer is not waiting on us, never that we are
// waiting on them.

import type { components } from "../../../api/schema";

type DealStatusCard = components["schemas"]["DealStatusCard"];
type Activity = components["schemas"]["Activity"];

/**
 * When the unanswered message arrived, or null when nobody is waiting or the
 * day cannot be named.
 *
 * `next.evidence` carries the timestamp and comes from the same rule as
 * `reply_to`, so it is tried first. Otherwise the loaded timeline is searched
 * by id, which can miss: the page holds one page of rows. A miss answers null
 * rather than a guessed date.
 */
export function waitingSince(
  card: DealStatusCard,
  timeline: readonly Activity[],
): { at: string } | null {
  if (!card.reply_to) {
    return null;
  }
  const fromMove = card.next?.evidence?.find(
    (row) => row.activity_id === card.reply_to && row.occurred_at,
  );
  if (fromMove?.occurred_at) {
    return { at: fromMove.occurred_at };
  }
  const row = timeline.find((activity) => activity.id === card.reply_to);
  return row ? { at: row.occurred_at } : null;
}
