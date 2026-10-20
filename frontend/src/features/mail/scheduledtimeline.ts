// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { calendarDay } from "../../format/calendarday";
import { queueCounts, type ScheduledSend } from "./scheduledqueue";

// The waiting queue read as a timeline: one block per calendar day on the
// reader's own wall clock, earliest first. Pure and clock-free; the caller
// hands in `now`, so a test never races a real timer.

export type Day = Readonly<{
  /** `yyyy-mm-dd` on the reader's calendar. */
  day: string;
  sends: readonly ScheduledSend[];
}>;

/** Sends in send order, grouped by the reader's calendar day. */
export function dayGroups(
  sends: readonly ScheduledSend[],
  zone: string,
): readonly Day[] {
  const ordered = [...sends].sort(
    (a, b) => Date.parse(a.scheduled_at) - Date.parse(b.scheduled_at),
  );
  const days: { day: string; sends: ScheduledSend[] }[] = [];
  for (const send of ordered) {
    const day = calendarDay(new Date(send.scheduled_at), zone);
    const last = days.at(-1);
    if (last?.day === day) {
      last.sends.push(send);
    } else {
      days.push({ day, sends: [send] });
    }
  }
  return days;
}

/** Whether a calendar day is the reader's today or tomorrow, or neither. */
export function relativeDay(
  day: string,
  now: Date,
  zone: string,
): "today" | "tomorrow" | null {
  const today = calendarDay(now, zone);
  if (day === today) return "today";
  const [year = 0, month = 1, date = 1] = today.split("-").map(Number);
  const next = new Date(Date.UTC(year, month - 1, date + 1));
  return day === next.toISOString().slice(0, 10) ? "tomorrow" : null;
}

/**
 * The queue cut the three ways the page reads it, and the message that goes
 * next.
 *
 * `held` is first on the page because it is the only status that is waiting on
 * the sender: a gate refused at fire, or the moment passed while nothing was
 * running, and the message will not send itself. `waiting` is the queue proper.
 * `closed` is everything that will not change on its own (released, sent,
 * cancelled), kept because a rep who withdrew a message wants to see that it is
 * withdrawn rather than gone.
 */
export function readQueue(
  sends: readonly ScheduledSend[],
  now: Date,
  zone: string,
) {
  const held = sends.filter((send) => send.status === "held");
  const waiting = sends.filter((send) => send.status === "scheduled");
  const closed = sends.filter(
    (send) => send.status !== "held" && send.status !== "scheduled",
  );
  const today = calendarDay(now, zone);
  return {
    held,
    waiting,
    closed,
    goingToday: waiting.filter(
      (send) => calendarDay(new Date(send.scheduled_at), zone) === today,
    ),
    next: dayGroups(waiting, zone)[0]?.sends[0],
    counts: queueCounts(sends),
  };
}
