// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  loadedQueue,
  type Worklist,
  type WorklistItem,
} from "./worklist.queries";

type Source = WorklistItem["source"];

/** Preserve the server's order across the pages the reader has requested. */
export function briefDay(
  pages: readonly Worklist[] | undefined,
): Worklist | undefined {
  const first = pages?.[0];
  if (!first || !pages) return undefined;
  const queue = loadedQueue(pages);
  const counts = first.counts.map((entry) => ({
    ...entry,
    shown: queue
      .filter((item) => item.category === entry.category)
      .reduce((total, item) => total + (item.batch?.count ?? 1), 0),
  }));
  return { ...first, queue, counts, next_cursor: pages.at(-1)?.next_cursor };
}

export function sourceComplete(day: Worklist, source: Source): boolean {
  if (day.sources_unavailable.some((entry) => entry.source === source))
    return false;
  const reach = day.reach?.find((entry) => entry.source === source);
  if (reach?.more_available) return false;
  if (reach) {
    return (
      day.queue.filter((item) => item.source === source).length >=
      reach.considered
    );
  }
  // Older responses without source coverage cannot certify a partial page.
  return (
    day.reach !== undefined ||
    (!day.next_cursor && !day.readings.more_available)
  );
}

export function scheduledMeetings(day: Worklist | undefined): WorklistItem[] {
  return day?.queue.filter((item) => item.source === "meeting") ?? [];
}

export function meetingReadiness(
  item: WorklistItem,
): "prepared" | "unprepared" | "unknown" {
  if (item.kind === "prepared") return "prepared";
  if (item.because.some((reason) => reason.kind === "meeting_unprepared"))
    return "unprepared";
  return "unknown";
}
