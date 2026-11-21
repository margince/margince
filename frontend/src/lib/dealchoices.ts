// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type DealSeat = components["schemas"]["Contact360DealRole"];

/** One open deal a message to a contact can be about. */
export type DealChoice = Readonly<{ id: string; name: string }>;

// The deals a follow-up to this contact can rest on, once each: an open deal
// on which the contact's seat has not ended. That is the engine's own live
// stakeholder rule (consent liveStakeholderSQL), so a deal offered here is one
// the check can read. A deal with two seats for one contact is still one deal.
export function dealChoicesOf(seats: readonly DealSeat[]): DealChoice[] {
  const choices: DealChoice[] = [];
  for (const seat of seats) {
    if (seat.deal_status !== "open" || seat.seat_current !== true) continue;
    if (choices.some((choice) => choice.id === seat.deal_id)) continue;
    choices.push({ id: seat.deal_id, name: seat.deal_title ?? "" });
  }
  return choices;
}
