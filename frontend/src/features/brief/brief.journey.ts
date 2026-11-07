// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { JourneyMark } from "../../lib/journeymarks";
import { receiptGaps, worklistGaps } from "./brief.completeness";

/**
 * The journey marks Home has reached.
 *
 * Useful: the day's work is on screen, rows or a confirmed empty day.
 * Complete: the brief, the handled receipts and the overnight receipt have
 * answered too, and nothing they hold is a gap (brief.completeness.ts).
 * Degraded: they answered with a gap, so the page cannot claim it is complete.
 */
export function homeJourneyMarks(home: {
  state: "loading" | "failed" | "ready";
  day: components["schemas"]["Worklist"] | undefined;
  settled: boolean;
  receipt: components["schemas"]["MagicReceipt"] | undefined;
}): JourneyMark[] {
  if (home.state === "failed") return ["home:failed"];
  if (home.state !== "ready" || !home.day) return [];
  if (!home.settled) return ["home:useful"];
  const gaps = [
    ...worklistGaps(home.day),
    ...(home.receipt ? receiptGaps(home.receipt) : []),
  ];
  return ["home:useful", gaps.length > 0 ? "home:degraded" : "home:complete"];
}
