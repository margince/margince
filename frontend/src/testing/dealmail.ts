// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { BoardDealMail } from "../design-system/dealcard";

// The instant a fixture's last email is measured from, so its age is fixed.
const MEASURED_AT = Date.parse("2026-09-15T12:00:00Z");

/** A deal's last email, sent `agoMs` before the card was built. */
export function lastEmailAgo(
  agoMs: number,
  direction: BoardDealMail["direction"],
): BoardDealMail {
  return {
    at: new Date(MEASURED_AT - agoMs).toISOString(),
    precision: "instant",
    measuredAt: MEASURED_AT,
    direction,
  };
}
