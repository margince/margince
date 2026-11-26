// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { calendarDay } from "../format/calendarday";
import { startOfDayInZone } from "../format/timezone";
import { useT } from "../i18n";
import { taskDueChoice, taskDueFields } from "../lib/activitybody";
import { useViewerZone } from "../lib/viewerzone";
import { useCompleteTask } from "./taskcomplete";
import { currentDueFields, useMoveTask } from "./taskduedatepick";
import type { useTaskUpdate } from "./taskwrite";

/** Keep the agreed wall time while moving at least as far as tomorrow. */
export function snoozedDueAt(
  dueAt: string | null | undefined,
  hasTime: boolean | undefined,
  zone: string,
  now = new Date(),
  intent?: components["schemas"]["IntentTime"],
): ReturnType<typeof taskDueFields> | null {
  if (!dueAt) return null;
  const choice = taskDueChoice(dueAt, hasTime, zone, intent);
  const dueDay = choice.day;
  const today = calendarDay(now, choice.zone);
  const base = dueDay > today ? dueDay : today;
  const next = calendarDay(
    new Date(startOfDayInZone(base, choice.zone, 1)),
    choice.zone,
  );
  return taskDueFields(next, choice.time, choice.zone);
}

/** Complete, or snooze a dated task. */
export function TaskQuickActions({
  activityId,
  version,
  dueAt,
  dueHasTime,
  dueIntent,
  update,
  showComplete = true,
  completeLabel,
  quiet = false,
}: Readonly<{
  activityId: string;
  version: number | undefined;
  dueAt?: string | null;
  /** The task's `due_has_time`: whether `dueAt` is a time of day. */
  dueHasTime?: boolean;
  dueIntent?: components["schemas"]["IntentTime"];
  update: ReturnType<typeof useTaskUpdate>;
  showComplete?: boolean;
  completeLabel?: string;
  /** Done steps back to a plain button beside another filled verb. */
  quiet?: boolean;
}>) {
  const t = useT();
  const readerZone = useViewerZone();
  // Any task write through this `update` holds every verb. The page draws the
  // old next task until the reads land; a press then acts on a stale row.
  const pending = update.isPending;
  const complete = useCompleteTask(update);
  const move = useMoveTask(update);
  return (
    <>
      {showComplete && (
        <Button
          variant={quiet ? undefined : "primary"}
          disabled={pending}
          onClick={() => complete(activityId, version)}
        >
          {completeLabel ?? t("tasks.complete")}
        </Button>
      )}
      {dueAt && (
        <Button
          disabled={pending}
          onClick={() => {
            const nextDue = snoozedDueAt(
              dueAt,
              dueHasTime,
              readerZone,
              new Date(),
              dueIntent,
            );
            if (nextDue)
              void move({
                id: activityId,
                version,
                to: nextDue,
                back: currentDueFields(
                  dueAt,
                  dueHasTime,
                  readerZone,
                  dueIntent,
                ),
              });
          }}
        >
          {t("tasks.snooze")}
        </Button>
      )}
    </>
  );
}
