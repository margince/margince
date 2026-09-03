// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who a first message from a deal page is addressed to.
//
// A deal has SEVERAL stakeholders, each with a buying role, so unlike a lead —
// whose address is on the record — this is a choice. The rule is the one a rep
// makes by hand: write to the champion; failing that, to somebody the deal is
// actually in conversation with; failing that, to the first seat on it.
//
// It only ever OFFERS. The composer fills an empty To field once and never over
// what the reader typed, so a wrong guess here costs one deletion rather than a
// misdirected message — which is what makes offering better than an empty field
// on a deal whose champion is sitting in the rail.

import type { components } from "../../api/schema";

type DealCoverageSeat = components["schemas"]["DealCoverageSeat"];

// The seat a rep writes to when the deal has one. Recorded, never inferred from
// a job title — the contract says so where the role is declared.
const CHAMPION = "champion";

/**
 * dealRecipientSeat picks the stakeholder a first message goes to, or undefined
 * when the deal has nobody to offer.
 *
 * A seat whose `person_name` is null is skipped at every step. That null means
 * the caller may not read that person: the seat still counts toward coverage —
 * how many people carry a deal is not a fact being withheld — but addressing a
 * message to somebody this reader cannot open would put a name in their To
 * field that the rest of the product refuses to show them.
 *
 * `engaged` is the second preference because it means a two-way exchange
 * happened in the window. A seat somebody has actually spoken with is a better
 * guess than one recorded and never contacted.
 */
export function dealRecipientSeat(
  seats: readonly DealCoverageSeat[] | undefined,
): DealCoverageSeat | undefined {
  if (!seats) {
    return undefined;
  }
  const readable = seats.filter((seat) => seat.person_name);
  return (
    readable.find((seat) => seat.role === CHAMPION) ??
    readable.find((seat) => seat.engaged) ??
    readable[0]
  );
}
