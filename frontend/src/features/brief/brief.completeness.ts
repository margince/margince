// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type Worklist = components["schemas"]["Worklist"];
type MagicReceipt = components["schemas"]["MagicReceipt"];

/**
 * Why the day's answer cannot make Home complete, or [] when it can. Home's
 * journey marks and the Home benchmark both judge by this one rule.
 *
 * A source that failed is missing work, and so is a day with no `focus`,
 * because the lead card is drawn from it. A bounded read is not a gap:
 * `more_available` on a count or the readings and `scope_truncated` say there
 * is more than the page holds, which Home draws as a floor ("5+").
 */
export function worklistGaps(day: Worklist): string[] {
  const gaps = failedSources(day.sources_unavailable).map(
    (source) => `worklist: source ${source} failed`,
  );
  if (day.focus === undefined) gaps.push("worklist: no focus to lead with");
  return gaps;
}

/** Why the overnight receipt cannot make Home complete, or []. */
export function receiptGaps(receipt: MagicReceipt): string[] {
  return failedSources(receipt.sources_unavailable).map(
    (source) => `magic: source ${source} failed`,
  );
}

// A withheld source is the reader's grant, true on every read, so it leaves
// the page complete.
function failedSources(missing: Worklist["sources_unavailable"]): string[] {
  return missing
    .filter((entry) => entry.reason === "failed")
    .map((entry) => entry.source);
}
