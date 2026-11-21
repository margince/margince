// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A new moment for a message already scheduled or held: a day and a time in
// the message's own zone. Rescheduling and Send anyway at a later time both
// pick through here, so the two cannot read one choice in two zones.

import { useState } from "react";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { type ClockTime, TimeInput } from "../../design-system/timeinput";
import { type ScheduleFields, scheduleFields } from "./composeschedule";
import type { ScheduledSend } from "./scheduledqueue";

/** The zone a message's new moment is read in: the one it was set in, else the reader's. */
export function messageZone(
  send: Pick<ScheduledSend, "scheduled_intent" | "scheduled_tz">,
  readerZone: string,
): string {
  return send.scheduled_intent?.timezone ?? send.scheduled_tz ?? readerZone;
}

/** The day and time being picked, and the moment they make in `zone`. */
export function useMomentChoice(zone: string, now: Date) {
  const [day, setDay] = useState<ISODate | "">("");
  const [time, setTime] = useState<ClockTime | "">("");
  const fields: ScheduleFields =
    day && time ? scheduleFields(`${day}T${time}`, undefined, zone) : {};
  const past =
    fields.scheduled_at !== undefined &&
    new Date(fields.scheduled_at).getTime() <= now.getTime();
  return {
    day,
    setDay,
    time,
    setTime,
    fields,
    /** A whole moment is picked and it is in the past. */
    past,
    /** A whole moment is picked and it is still ahead. */
    ready: fields.scheduled_at !== undefined && !past,
  };
}

export type MomentChoice = ReturnType<typeof useMomentChoice>;

export function MomentFields({
  choice,
  dayLabel,
  timeLabel,
}: Readonly<{ choice: MomentChoice; dayLabel: string; timeLabel: string }>) {
  return (
    <>
      <DateInput
        aria-label={dayLabel}
        value={choice.day}
        onChange={(event) => choice.setDay(event.target.value)}
      />
      <TimeInput
        aria-label={timeLabel}
        value={choice.time}
        onChange={(event) => choice.setTime(event.target.value)}
      />
    </>
  );
}
