// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDateAbbrev } from "../format/format";
import { viewerZone } from "../format/timezone";
import type { Locale } from "../i18n";
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
    more_available: pages.some((page) =>
      page.counts.some(
        (count) => count.category === entry.category && count.more_available,
      ),
    ),
    shown: queue
      .filter((item) => item.category === entry.category)
      .reduce((total, item) => total + (item.batch?.count ?? 1), 0),
  }));
  const unavailable = new Map(
    pages
      .flatMap((page) => page.sources_unavailable)
      .map((entry) => [entry.source, entry]),
  );
  return {
    ...first,
    queue,
    counts,
    sources_unavailable: [...unavailable.values()],
    readings: {
      ...first.readings,
      more_available: pages.some((page) => page.readings.more_available),
    },
    next_cursor: pages.at(-1)?.next_cursor,
  };
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

/** How many meetings the day holds: every one ranked, else every row carried. */
export function meetingsCounted(day: Worklist): number {
  return (
    day.reach?.find((entry) => entry.source === "meeting")?.considered ??
    scheduledMeetings(day).length
  );
}

type NextMeeting = NonNullable<Worklist["next_meeting"]>;

/**
 * What today's calendar lets the page say, read once for the readings strip
 * and the schedule panel so one morning cannot be told two ways.
 *
 * `counted` keeps the figure as it stands; `unread` is a meetings lane that
 * never answered. `not_connected` and `unreadable` are a zero that measured
 * nothing, so neither surface may draw it as a day without meetings, though a
 * meeting booked by hand is still a fact the CRM can name. `quiet` is a zero
 * that did measure, with the next booked conversation to name.
 */
export type CalendarDay =
  | Readonly<{ state: "counted" | "unread" }>
  | Readonly<{ state: "not_connected" | "unreadable"; next?: NextMeeting }>
  | Readonly<{ state: "quiet"; next: NextMeeting }>;

/** The next meeting's day, spelled one way wherever the brief names it. */
export function nextMeetingDay(next: NextMeeting, locale: Locale): string {
  return formatDateAbbrev(next.starts_at, locale, viewerZone());
}

export function calendarDay(day: Worklist): CalendarDay {
  if (day.sources_unavailable.some((entry) => entry.source === "meeting")) {
    return { state: "unread" };
  }
  if (meetingsCounted(day) > 0) {
    return { state: "counted" };
  }
  if (day.calendar === "not_connected" || day.calendar === "unreadable") {
    return { state: day.calendar, next: day.next_meeting };
  }
  // No `calendar` is a server that cannot say whether its zero measured
  // anything, so the reading stays what it always was.
  if (day.calendar === "connected" && day.next_meeting) {
    return { state: "quiet", next: day.next_meeting };
  }
  return { state: "counted" };
}
