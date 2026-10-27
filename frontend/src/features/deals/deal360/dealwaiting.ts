// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import {
  type ActivityMoment,
  activityDaysAway,
} from "../../../format/activitywhen";

type DealStatusCard = components["schemas"]["DealStatusCard"];

/** Source time for a confirmed current request in this deal's scope. */
export function currentReplyTarget(card: DealStatusCard): string | null {
  const basis = card.request_basis;
  if (
    !card.reply_to ||
    basis?.kind !== "current_request" ||
    basis.scope !== "deal" ||
    basis.source_activity_id !== card.reply_to ||
    (basis.source_precision !== "day" &&
      basis.source_precision !== "instant") ||
    !Number.isFinite(Date.parse(basis.source_at))
  ) {
    return null;
  }
  return card.reply_to;
}

export function waitingSince(card: DealStatusCard): ActivityMoment | null {
  const basis = card.request_basis;
  return currentReplyTarget(card) && basis
    ? { at: basis.source_at, precision: basis.source_precision }
    : null;
}

/** How many of the record zone's days the buyer has waited, as of `asOf`. */
export function daysWaiting(
  { at, precision }: ActivityMoment,
  asOf: string,
  record: string,
): number {
  return Math.max(
    0,
    -activityDaysAway(at, precision, Date.parse(asOf), record, record),
  );
}
