// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../../format/format";
import type { Locale, Translator, usePlural } from "../../i18n";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import type { Worklist } from "../worklist/worklist.queries";
import {
  type CalendarDay,
  meetingReadiness,
  meetingsCounted,
  nextMeetingDay,
  scheduledMeetings,
  sourceComplete,
} from "./brief.facts";

// The meetings slot of the readings strip: its figure, its line, and what
// stands in for the figure when the calendar behind it could not count.

/** A word where a figure would stand, and a door to what would produce one. */
export type StandIn = Readonly<{
  value: string;
  detail: string;
  open: () => void;
}>;

export type MeetingsReading = Readonly<{
  meetings: number | null;
  // Null when the page carries fewer meetings than it counted, so no readiness
  // figure can be stated. That is not the same as zero unprepared.
  unready: number | null;
}>;

// The meetings reading: how many stand behind the day, and how many of those
// nothing is prepared for, or that the second question could not be answered.
//
// The two figures come from different populations and that is the whole care
// here. `considered` counts every meeting read and ranked, before the fold and
// before the page cut; the readiness figure can only be counted off the rows the
// page actually carries. Divide one by the other and a day with ten meetings
// considered and three on the page reads "10 · 2 need prep", telling a rep eight
// meetings are ready when nothing checked them.
//
// So readiness is claimed only when the page carries every meeting it counted.
export function meetingsReading(
  day: Worklist,
  calendar: CalendarDay,
): MeetingsReading {
  const meetings = scheduledMeetings(day);
  if (calendar.state === "unread") {
    return { meetings: null, unready: null };
  }
  const known =
    sourceComplete(day, "meeting") &&
    meetings.every((item) => meetingReadiness(item) !== "unknown");
  return {
    meetings: meetingsCounted(day),
    unready: known
      ? meetings.filter((item) => meetingReadiness(item) === "unprepared")
          .length
      : null,
  };
}

// Meetings booked by hand still count while the calendar cannot be read. The
// figure is then a minimum, and the line says how to make it whole.
export function calendarCut(day: Worklist, calendar: CalendarDay): boolean {
  return calendar.state === "counted" && day.calendar === "unreadable";
}

/** The line under the meetings figure: when the next meeting is on a quiet
 *  day, how to make a cut figure whole, or how many meetings are prepared. */
export function meetingsBasis(
  day: Worklist,
  calendar: CalendarDay,
  reading: MeetingsReading,
  say: Readonly<{
    locale: Locale;
    t: Translator;
    plural: ReturnType<typeof usePlural>;
    zone: string;
  }>,
): string | undefined {
  if (calendar.state === "quiet") {
    return nextMeetingLine(calendar.next, say.locale, say.t, say.zone);
  }
  if (calendarCut(day, calendar)) {
    return say.t("brief.readings.calendarUnreadableWhy");
  }
  return meetingsDetail(reading, say.locale, say.t, say.plural);
}

/** Whether the meetings figure is a minimum rather than a count. */
export function meetingsFloor(day: Worklist, calendar: CalendarDay): boolean {
  return (
    calendarCut(day, calendar) ||
    day.reach?.find((entry) => entry.source === "meeting")?.more_available ===
      true ||
    day.sources_unavailable.some(
      (entry) => entry.source === "meeting" || !entry.category,
    )
  );
}

// How many meetings, and how many of them nothing is prepared for.
//
// Readiness is the fact that changes what a reader does before the first one
// starts, so a day with meetings and nothing unprepared says "all prepared"
// rather than leaving the line blank: the absence of a warning has to be
// readable as an answer, not as a gap.
export function meetingsDetail(
  reading: MeetingsReading,
  locale: Locale,
  t: Translator,
  plural: ReturnType<typeof usePlural>,
): string {
  const { meetings, unready } = reading;
  if (unready === null) {
    return t("brief.readings.prepUnknown");
  }
  if (unready > 0) {
    return plural("brief.readings.needsPrep", unready, {
      count: formatNumber(unready, locale),
    });
  }
  // "All prepared" is a claim about meetings, and an empty day has none to make
  // it about. The basis line says what was looked at instead.
  return meetings === 0
    ? t("brief.readings.meetingsBasis")
    : t("brief.readings.prepared");
}

export function nextMeetingLine(
  next: NonNullable<Worklist["next_meeting"]>,
  locale: Locale,
  t: Translator,
  zone: string,
): string {
  const date = nextMeetingDay(next, locale, zone);
  return next.subject
    ? t("brief.readings.nextMeeting", { date, subject: next.subject })
    : t("brief.readings.nextMeetingUntitled", { date });
}

// A calendar that could not count leads to where it is connected, because
// that is the one thing a reader can do about a zero that measured nothing.
export function calendarStandIn(
  calendar: CalendarDay,
  t: Translator,
): StandIn | undefined {
  const open = () => navigate(settingsHref("connections"));
  if (calendar.state === "not_connected") {
    return {
      value: t("brief.readings.calendarNotConnected"),
      detail: t("brief.readings.calendarNotConnectedWhy"),
      open,
    };
  }
  if (calendar.state === "unreadable") {
    return {
      value: t("brief.readings.calendarUnreadable"),
      detail: t("brief.readings.calendarUnreadableWhy"),
      open,
    };
  }
  return undefined;
}
