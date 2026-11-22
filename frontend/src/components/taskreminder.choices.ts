// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import {
  calendarDay,
  dueInstant,
  shiftCalendarDay,
} from "../format/calendarday";

type IntentTime = components["schemas"]["IntentTime"];

/** What one reminder write sends: the instant, and the wall clock picked. */
export type ReminderWrite = { remind_at: string; remind_intent?: IntentTime };

export type ReminderChoice = Readonly<{
  key: "inAnHour" | "tomorrowMorning" | "beforeDue";
  write: ReminderWrite;
}>;

const HOUR_MS = 60 * 60 * 1000;

/** The hour "tomorrow morning" means on the reader's own clock. */
export const MORNING = "09:00";

/** The reminder at `time` on `day` in `zone`, kept as that wall clock. */
export function reminderAtWallClock(
  day: string,
  time: string,
  zone: string,
): ReminderWrite {
  return {
    remind_at: dueInstant(day, zone, time),
    remind_intent: { wall_clock: `${day}T${time}`, timezone: zone },
  };
}

/**
 * The quick choices a task's Remind me menu offers at `nowMs`. "Before it is
 * due" needs a due time of day more than an hour away. A day-only deadline has
 * no hour to come before, and a past instant would remind at once.
 */
export function reminderChoices(
  nowMs: number,
  zone: string,
  due: Readonly<{ at?: string | null; hasTime?: boolean }>,
): ReminderChoice[] {
  const tomorrow = shiftCalendarDay(calendarDay(new Date(nowMs), zone), 1);
  const choices: ReminderChoice[] = [
    {
      key: "inAnHour",
      write: { remind_at: new Date(nowMs + HOUR_MS).toISOString() },
    },
    {
      key: "tomorrowMorning",
      write: reminderAtWallClock(tomorrow, MORNING, zone),
    },
  ];
  const beforeDue = due.at && due.hasTime ? Date.parse(due.at) - HOUR_MS : NaN;
  if (beforeDue > nowMs) {
    choices.push({
      key: "beforeDue",
      write: { remind_at: new Date(beforeDue).toISOString() },
    });
  }
  return choices;
}

/** The quick choice `key` as it stands at the moment of the press, if still offered. */
export function reminderChoiceNow(
  key: ReminderChoice["key"],
  zone: string,
  due: Readonly<{ at?: string | null; hasTime?: boolean }>,
): ReminderChoice | undefined {
  return reminderChoices(Date.now(), zone, due).find(
    (choice) => choice.key === key,
  );
}
