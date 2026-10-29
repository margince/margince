// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { JourneyMark } from "../../lib/journeymarks";
import type { MagicReceipt } from "../../lib/magic.queries";
import type { Worklist } from "../worklist/worklist.queries";

/**
 * The journey marks Home has reached.
 *
 * Useful: the day's work is on screen, rows or a confirmed empty day.
 * Complete: the brief, the handled receipts and the overnight receipt have
 * answered too, and no answer names a source that failed. Degraded: they
 * answered, but a source failed, so the page cannot claim it is complete. A
 * withheld source is the reader's grant and leaves the page complete.
 */
export function homeJourneyMarks(home: {
  state: "loading" | "failed" | "ready";
  day: Worklist | undefined;
  settled: boolean;
  receipt: MagicReceipt | undefined;
}): JourneyMark[] {
  if (home.state === "failed") return ["home:failed"];
  if (home.state !== "ready" || !home.day) return [];
  if (!home.settled) return ["home:useful"];
  const missing = [
    ...home.day.sources_unavailable,
    ...(home.receipt?.sources_unavailable ?? []),
  ];
  const failed = missing.some((entry) => entry.reason === "failed");
  return ["home:useful", failed ? "home:degraded" : "home:complete"];
}
