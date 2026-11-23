// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Field } from "../design-system/atoms";
import { DateInput } from "../design-system/dateinput";
import { dueDayBounds } from "../format/calendarday";
import { isoOrBlank } from "../format/datepattern";
import { useT } from "../i18n";
import { taskDueChoice, taskDueFields } from "../lib/activitybody";
import { useViewerZone } from "../lib/viewerzone";
import { TaskTimeField } from "./followuptaskfields";
import type { useTaskUpdate } from "./taskwrite";

/**
 * The day and time to move a task to. The detail modal draws it, where a
 * reader came to work on one task; a date box in every row is too heavy.
 * It also dates a task that has no deadline to snooze.
 */
export function TaskDueDatePick({
  activityId,
  version,
  dueAt,
  dueHasTime,
  dueIntent,
  update,
}: Readonly<{
  activityId: string;
  version: number | undefined;
  dueAt?: string | null;
  dueHasTime?: boolean;
  dueIntent?: components["schemas"]["IntentTime"];
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const pending = update.isPending;
  const readerZone = useViewerZone();
  // Seeded from the task's own day and time in the zone the meta line reads
  // it in. A picker opening on another day would answer "when is it due" twice.
  const choice = taskDueChoice(dueAt, dueHasTime, readerZone, dueIntent);
  const current = isoOrBlank(choice.day);
  const time = choice.time;
  const move = (day: string, nextTime: string) =>
    update.mutate({
      id: activityId,
      version,
      body: taskDueFields(day, nextTime, choice.zone),
    });
  return (
    <>
      <Field label={t("tasks.moveTo")}>
        {(field) => (
          <DateInput
            {...field}
            value={current}
            {...dueDayBounds(readerZone)}
            disabled={pending}
            onChange={(event) => {
              const day = event.target.value;
              // A cleared box is not a request to undate the task.
              if (day === "") {
                return;
              }
              move(day, time);
            }}
          />
        )}
      </Field>
      {/* Clearing the time puts the task back to the end of its day. */}
      <TaskTimeField
        day={pending ? "" : current}
        value={time}
        onChange={(nextTime) => {
          if (current !== "") move(current, nextTime);
        }}
      />
    </>
  );
}
