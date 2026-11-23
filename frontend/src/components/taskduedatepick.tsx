// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DateInput } from "../design-system/dateinput";
import { undoAction, useToast } from "../design-system/toast";
import { dueDayBounds } from "../format/calendarday";
import { isoOrBlank } from "../format/datepattern";
import { formatDueCompact } from "../format/format";
import { useNow } from "../format/now";
import { useLocale, useT } from "../i18n";
import { taskDueChoice, taskDueFields } from "../lib/activitybody";
import { useViewerZone } from "../lib/viewerzone";
import { dueDayChange, TaskTimeField } from "./followuptaskfields";
import type { useTaskUpdate } from "./taskwrite";

type DueFields = ReturnType<typeof taskDueFields>;

/** One move of a task's deadline, and the deadline an Undo puts back. */
export type TaskMove = Readonly<{
  id: string;
  version: number | undefined;
  to: DueFields;
  /** Absent for a task that had no deadline: the write cannot take one away. */
  back?: DueFields;
}>;

/** The deadline a task has now, as the write that would put it back. */
export function currentDueFields(
  dueAt: string | null | undefined,
  dueHasTime: boolean | undefined,
  zone: string,
  dueIntent?: components["schemas"]["IntentTime"],
): DueFields | undefined {
  if (!dueAt) return undefined;
  const choice = taskDueChoice(dueAt, dueHasTime, zone, dueIntent);
  return taskDueFields(choice.day, choice.time, choice.zone);
}

/**
 * Moving a task's deadline from any verb: the confirmation names the new
 * deadline and its Undo puts the old one back, as Done's does. The promise
 * answers whether the task moved; a refusal stays on `update`.
 */
export function useMoveTask(update: ReturnType<typeof useTaskUpdate>) {
  const t = useT();
  const toast = useToast();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  return (move: TaskMove) =>
    update
      .mutateAsync({ id: move.id, version: move.version, body: move.to })
      .then((movedAt) => {
        const { back } = move;
        const when = formatDueCompact(
          move.to.due_at,
          move.to.due_has_time,
          locale,
          readerZone,
          move.to.due_intent,
        );
        toast.show(t("tasks.movedTo", { when }), {
          // Pinned on the version the move produced, which the older one
          // would be refused against.
          action: back
            ? undoAction(t("common.undo"), () => {
                update
                  .mutateAsync({ id: move.id, version: movedAt, body: back })
                  .catch(() =>
                    toast.show(t("tasks.moveUndoFailed"), { tone: "danger" }),
                  );
              })
            : undefined,
        });
        return true;
      })
      .catch(() => false);
}

// What the reader picked, with the version and deadline it was picked against.
type MoveDraft = Readonly<{
  day: string;
  time: string;
  version: number | undefined;
  back?: DueFields;
}>;

/**
 * The day and time to move a task to, sent together by its Move button. The
 * detail modal draws it, where a reader came to work on one task; a date box
 * in every row is too heavy. It also dates a task that has no deadline.
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
  const now = useNow(60_000);
  const move = useMoveTask(update);
  // Seeded from the task's own day and time in the zone the meta line reads
  // it in. A picker opening on another day would answer "when is it due" twice.
  const choice = taskDueChoice(dueAt, dueHasTime, readerZone, dueIntent);
  const [draft, setDraft] = useState<MoveDraft | null>(null);
  const day = draft?.day ?? choice.day;
  const time = draft?.time ?? choice.time;
  const edit = (change: Readonly<{ day?: string; time?: string }>) =>
    setDraft((held) => ({
      day: held?.day ?? choice.day,
      time: held?.time ?? choice.time,
      version: held?.version ?? version,
      back:
        held === null
          ? currentDueFields(dueAt, dueHasTime, readerZone, dueIntent)
          : held.back,
      ...change,
    }));
  const to =
    draft !== null && day !== "" && (day !== choice.day || time !== choice.time)
      ? taskDueFields(day, time, choice.zone)
      : null;
  const past = to !== null && Date.parse(to.due_at) < now;
  return (
    <>
      <div className="task-detail-due">
        <Field label={t("tasks.moveTo")}>
          {(field) => (
            <DateInput
              {...field}
              value={isoOrBlank(day)}
              {...dueDayBounds(readerZone)}
              disabled={pending}
              onChange={(event) => edit(dueDayChange(event.target.value))}
            />
          )}
        </Field>
        {/* Clearing the time puts the task back to the end of its day. */}
        <TaskTimeField
          day={pending ? "" : day}
          value={time}
          onChange={(nextTime) => edit({ time: nextTime })}
        />
        <Button
          disabled={to === null}
          pending={pending && to !== null}
          onClick={() => {
            if (draft === null || to === null) return;
            void move({
              id: activityId,
              version: draft.version,
              to,
              back: draft.back,
            }).then((moved) => {
              if (moved) setDraft(null);
            });
          }}
        >
          {past ? t("tasks.moveAnyway") : t("tasks.move")}
        </Button>
      </div>
      {past && (
        <Callout tone="warning" kind="event" title={t("tasks.movePastTitle")}>
          {t("tasks.movePastBody")}
        </Callout>
      )}
    </>
  );
}
