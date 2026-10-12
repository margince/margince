// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { type ISODay, isoDay } from "../design-system/calendar";
import { stable } from "../format/collate";
import { dayInZone, startOfDayInZone } from "../format/timezone";

type Availability = components["schemas"]["MeetingAvailability"];
export type GuestSlot = Availability["slots"][number];

// A truncated answer is followed from its last time, a bounded number of
// times, so a busy host's month is not cut off after its first days.
const MAX_PAGES = 4;
const STEP_PAST_LAST = 15 * 60000;
// The server answers at most 31 days at a time, and a month with a fall-back
// clock change is an hour longer than that in the guest's zone.
const MAX_SPAN = 31 * 86400000;

// `dayInZone` spells a day as the calendar does; this lets the type say so.
function isCalendarDay(value: string): value is ISODay {
  return /^\d{4}-\d{2}-\d{2}$/.test(value);
}

/** The `yyyy-mm` a grid's month is, which every day in it starts with. */
export function isoMonth(month: Date): string {
  return isoDay(month).slice(0, 7);
}

/** The first of the month a chosen day belongs to, as the grid holds months. */
export function monthOf(day: ISODay): Date {
  const [year, month] = day.split("-").map(Number);
  return new Date(year, month - 1, 1);
}

/**
 * The instants a month on show covers in the guest's zone, starting no earlier
 * than now: null for a month that is already over.
 */
export function monthWindow(month: Date, zone: string, now: number) {
  const first = isoDay(new Date(month.getFullYear(), month.getMonth(), 1));
  const next = isoDay(new Date(month.getFullYear(), month.getMonth() + 1, 1));
  const to = startOfDayInZone(next, zone);
  if (Date.parse(to) <= now) return null;
  const start = startOfDayInZone(first, zone);
  const from = Date.parse(start) > now ? start : new Date(now).toISOString();
  return { from, to };
}

/** The instants one day covers in the guest's zone, starting no earlier than now. */
export function dayWindow(day: ISODay, zone: string, now: number) {
  const [year, month, date] = day.split("-").map(Number);
  const to = startOfDayInZone(
    isoDay(new Date(year, month - 1, date + 1)),
    zone,
  );
  const start = startOfDayInZone(day, zone);
  const from = Date.parse(start) > now ? start : new Date(now).toISOString();
  return { from, to };
}

/** Every free time in one window, however many pages the server answers in. */
export async function readMonth(
  read: (from: string, to: string) => Promise<Availability>,
  window: Readonly<{ from: string; to: string }>,
): Promise<Availability> {
  const byStart = new Map<string, GuestSlot>();
  let cursor = window.from;
  for (let page = 0; page < MAX_PAGES; page++) {
    const spanEnd = Date.parse(cursor) + MAX_SPAN;
    const to =
      spanEnd < Date.parse(window.to)
        ? new Date(spanEnd).toISOString()
        : window.to;
    const answer = await read(cursor, to);
    for (const slot of answer.slots) byStart.set(slot.start, slot);
    const last = answer.slots.at(-1);
    if (answer.truncated && last)
      cursor = new Date(Date.parse(last.start) + STEP_PAST_LAST).toISOString();
    else if (to !== window.to) cursor = to;
    else return { slots: [...byStart.values()], truncated: false };
  }
  return { slots: [...byStart.values()], truncated: true };
}

/**
 * The month's days as a guest can use them: which have a free time, and past
 * which day the answer stopped (a still-truncated read knows nothing after
 * its last time, so those days stay open rather than being refused).
 */
export function monthDays(
  availability: Availability | undefined,
  zone: string,
) {
  const slots = availability?.slots ?? [];
  const byDay = new Map<string, GuestSlot[]>();
  for (const slot of slots) {
    const key = dayInZone(Date.parse(slot.start), zone);
    byDay.set(key, [...(byDay.get(key) ?? []), slot]);
  }
  const last = slots.at(-1);
  const knownUntil =
    availability?.truncated && last
      ? dayInZone(Date.parse(last.start), zone)
      : undefined;
  const free = [...byDay.keys()].filter(isCalendarDay).sort(stable);
  return { byDay, free, knownUntil };
}

/**
 * A day the month's read did not finish, so its times need a read of their
 * own: the day the last page stopped in is as partial as any after it.
 */
export function pastKnown(
  day: ISODay | "",
  days: ReturnType<typeof monthDays> | undefined,
): day is ISODay {
  return day !== "" && !!days?.knownUntil && day >= days.knownUntil;
}

/** Why a day in the grid cannot be chosen, given what the month's read said. */
export function dayRefusal(
  day: ISODay,
  today: string,
  monthKey: string,
  days: ReturnType<typeof monthDays> | undefined,
): "past" | "full" | undefined {
  if (day < today) return "past";
  if (!days || !day.startsWith(monthKey)) return undefined;
  if (days.knownUntil && day > days.knownUntil) return undefined;
  return days.byDay.has(day) ? undefined : "full";
}
