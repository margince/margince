// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { activityDay } from "../../format/activitywhen";
import { formatDateAbbrev } from "../../format/format";
import type { Locale } from "../../i18n";
import { isUnprepared } from "../worklist/worklist.copy";
import type { Worklist, WorklistItem } from "../worklist/worklist.queries";

type Source = WorklistItem["source"];

export { accumulatedDay as briefDay } from "../worklist/worklist.reads";

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
  if (item.preparation?.state === "ready") return "prepared";
  if (isUnprepared(item)) return "unprepared";
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
export function nextMeetingDay(
  next: NextMeeting,
  locale: Locale,
  zone: string,
): string {
  return activityDay(
    next.starts_at,
    next.occurred_precision,
    locale,
    zone,
    formatDateAbbrev,
  );
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
