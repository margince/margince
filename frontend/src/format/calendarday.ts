// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Calendar days, as the reader keeps them. Two halves of ONE contract: the day
// a reader picks in a form, and the day a screen files an instant under. They
// agree only while both are read in the same zone, which is why they are spelled
// here together instead of once per screen — the tasks list grouped by UTC
// calendar day while the form minted its instant from local wall time, and every
// reader west of UTC watched a task they had just filed for today appear under
// Upcoming.
//
// Both are pure and take their zone (or the reader's own wall clock) as input,
// so nothing here has to be tested against the machine's own zone.

// The calendar day an instant falls on, in a named IANA zone, as `yyyy-mm-dd` so
// two of them compare as strings without a second parse.
//
// Assembled from the parts rather than from a locale whose short date happens to
// come out ISO-ordered. What the parts guarantee is the ORDER and the padding,
// which is the whole reason this string is comparable — a locale's date format is
// data, it varies with the runtime's ICU, and a bucketing rule that reads
// "yesterday" because a format changed under it would be invisible.
// One formatter per zone: constructing an Intl.DateTimeFormat is the expensive
// part, and a task list asks this question once per row.
const dayFormatters = new Map<string, Intl.DateTimeFormat>();

function dayFormatter(zone: string): Intl.DateTimeFormat {
  const cached = dayFormatters.get(zone);
  if (cached) {
    return cached;
  }
  const made = new Intl.DateTimeFormat("en-CA", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
  dayFormatters.set(zone, made);
  return made;
}

/**
 * Whether a `YYYY-MM-DD` string names a day that EXISTS.
 *
 * A shape test is not enough: `2026-02-30` and `2026-13-01` are both spelled
 * correctly and neither is a date. This compares the parts against what a real
 * calendar holds — which needs no clock and no zone at all, because the
 * question is about the STRING, not about whose calendar it lands on.
 *
 * It lives beside calendarDay because it is the same subject: what a calendar
 * day is, and which strings are one.
 */
export function isRealCalendarDay(day: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  if (!match) {
    return false;
  }
  const [year, month, dayOfMonth] = match.slice(1).map(Number);
  return (
    month >= 1 &&
    month <= 12 &&
    dayOfMonth >= 1 &&
    dayOfMonth <= daysIn(year, month)
  );
}

// How many days a month holds, February following the Gregorian leap rule
// (every four years, except centuries, except every four hundred).
function daysIn(year: number, month: number): number {
  if (month === 2) {
    const leap = (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
    return leap ? 29 : 28;
  }
  return [4, 6, 9, 11].includes(month) ? 30 : 31;
}

export function calendarDay(at: Date, zone: string): string {
  const parts = new Map(
    dayFormatter(zone)
      .formatToParts(at)
      .map((part) => [part.type, part.value]),
  );
  const [year, month, day] = [
    parts.get("year"),
    parts.get("month"),
    parts.get("day"),
  ];
  if (!year || !month || !day) {
    // Unreachable for the options above, and worth refusing rather than
    // returning "undefined-undefined-undefined", which would compare equal to
    // itself and quietly bucket every task into one day.
    throw new Error(`no calendar day for zone ${zone}`);
  }
  return `${year}-${month}-${day}`;
}

// The calendar MONTH an instant falls in, in a named IANA zone, as `yyyy-mm`.
//
// Derived from calendarDay rather than assembled again, so the two granularities
// cannot answer about different months. It exists because the day rule was
// spelled at month granularity by hand — `new Date().toISOString().slice(0, 7)`
// — and a census looking for the day-length slice could not see it: in the
// first hours of a new month east of UTC, where the reader has turned the page
// and UTC has not, that answers about UTC's month rather than the one on their
// calendar, and a usage page opens on a month they are no longer in.
export function calendarMonth(at: Date, zone: string): string {
  return calendarDay(at, zone).slice(0, "yyyy-mm".length);
}

// The wire instant for a due date the reader picked as a calendar day — the
// `yyyy-mm-dd` a date input yields, which every caller has already checked is
// non-empty.
//
// A task stays due until that day ends WHERE THE READER IS, so the instant is
// the local end of day. Midnight would file a task picked for today as overdue
// at breakfast, and `new Date(day)` on the bare date reads it as UTC midnight,
// which does the same thing a whole zone offset earlier — east of UTC that is
// overdue on waking, west of UTC it is the previous calendar day.
export function dueInstant(day: string): string {
  return new Date(`${day}T23:59:59`).toISOString();
}

// The wall-clock value a `datetime-local` input shows for an instant, in the
// reader's OWN zone, as the `yyyy-mm-ddThh:mm` that control accepts.
//
// It is the exact inverse of the composer's `scheduleFields`, which reads such a
// value back by handing it to `new Date(...)` and so resolves it against the
// browser's zone. The pair has to agree: a reschedule form seeded from a
// differently-zoned reading would open on a moment an hour off the one it is
// about to resubmit, and the reader would move a message they never moved.
//
// Assembled from the local getters rather than by slicing an ISO string.
// `toISOString()` is UTC, so west of UTC that would seed the picker with
// yesterday's date and an evening send would read as due the day before.
export function localDateTimeValue(utcIso: string): string {
  const at = new Date(utcIso);
  const pad = (value: number) => String(value).padStart(2, "0");
  const day = `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}`;
  return `${day}T${pad(at.getHours())}:${pad(at.getMinutes())}`;
}

// The instant that is NOON of a picked calendar day in a named zone — for a
// backdated entry whose screens render dates in that zone (the record pages'
// own record zone): filing it at the zone's own midday is what keeps it on the
// picked day both there and in the wall clock of any writer within twelve
// hours of that zone.
//
// Found without a timezone table: read what wall time the zone shows at UTC
// noon of that day, then shift the instant by the difference from the noon
// that was wanted. One correction is exact because a zone's offset does not
// change within the couple of hours the shift moves through — DST switches
// happen at night, and the target is midday.
export function middayInstant(day: string, zone: string): string {
  const utcNoon = new Date(`${day}T12:00:00Z`);
  const parts = new Map(
    new Intl.DateTimeFormat("en-CA", {
      timeZone: zone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      // h23, not `hour12: false`: some ICU builds resolve the latter to h24,
      // which prints midnight as "24" and breaks the ISO string below.
      hourCycle: "h23",
    })
      .formatToParts(utcNoon)
      .map((part) => [part.type, part.value]),
  );
  const shown = new Date(
    `${parts.get("year")}-${parts.get("month")}-${parts.get("day")}T${parts.get("hour")}:${parts.get("minute")}:00Z`,
  );
  const wanted = new Date(`${day}T12:00:00Z`);
  return new Date(
    utcNoon.getTime() - (shown.getTime() - wanted.getTime()),
  ).toISOString();
}
