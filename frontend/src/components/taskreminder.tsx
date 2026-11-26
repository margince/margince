// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Bell } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import type { components } from "../api/schema";
import { Button, Field, OverflowMenu } from "../design-system/atoms";
import { DateInput } from "../design-system/dateinput";
import { useCloseOverflowMenu } from "../design-system/overflowmenu";
import { RecordFormDialog } from "../design-system/recordformdialog";
import { useToast } from "../design-system/toast";
import { calendarDay, dueDayBounds } from "../format/calendarday";
import { isoOrBlank } from "../format/datepattern";
import { formatDue } from "../format/format";
import { useNow } from "../format/now";
import { useLocale, useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { useMe } from "../lib/session";
import { useViewerZone } from "../lib/viewerzone";
import { TaskTimeField } from "./followuptaskfields";
import {
  type ReminderChoice,
  type ReminderWrite,
  reminderAtWallClock,
  reminderChoiceNow,
  reminderChoices,
} from "./taskreminder.choices";
import type { useTaskUpdate } from "./taskwrite";

type Activity = components["schemas"]["Activity"];

const MINUTE_MS = 60_000;

/** The fields of a task its reminder lines read. */
export type ReminderTask = Pick<
  Activity,
  | "id"
  | "version"
  | "due_at"
  | "due_has_time"
  | "remind_at"
  | "remind_intent"
  | "assignee_id"
>;

/**
 * Whether `remind_at` would reach the reader. The server reminds the task's
 * assignee alone, so "Remind me" on a colleague's task would remind nobody.
 */
export function useRemindsReader(assigneeId: string | null | undefined) {
  const me = useMe().data?.user?.id;
  return me !== undefined && assigneeId === me;
}

/** The task detail's "Remind me": a chip in its line of facts that names the
 *  reminder once one stands, and opens the menu of the reminder lines. */
export function TaskRemindMenu({
  task,
  update,
}: Readonly<{
  task: ReminderTask;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  if (!useRemindsReader(task.assignee_id)) return null;
  const when = task.remind_at
    ? formatDue(task.remind_at, true, locale, zone, task.remind_intent)
    : undefined;
  // The chip reads the reminder, so its name carries the verb and that reading.
  return (
    <OverflowMenu
      label={
        when ? t("tasks.remind.chipName", { when }) : t("tasks.remind.menu")
      }
      text={when ? t("tasks.remind.current", { when }) : t("tasks.remind.menu")}
      face="chip"
      icon={<Bell size={12} aria-hidden="true" />}
      align="start"
      pending={update.isPending}
    >
      <TaskReminderLines task={task} update={update} />
    </OverflowMenu>
  );
}

/**
 * The reminder's lines, shared by the task detail and the Worklist row's
 * menu: the reminder standing now, three quick choices, Pick a time, Remove.
 */
export function TaskReminderLines({
  task,
  update,
}: Readonly<{
  task: ReminderTask;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const closeMenu = useCloseOverflowMenu();
  const remind = useReminderWrite(task, update);
  const [picking, setPicking] = useState(false);
  const due = { at: task.due_at, hasTime: task.due_has_time };
  // The minute decides which choices stand; the press reads the clock again,
  // so "In 1 hour" counts from the press and not from the last tick.
  const choices = reminderChoices(useNow(MINUTE_MS), zone, due);
  const choose = (write: ReminderWrite | null) => {
    closeMenu();
    void remind(write);
  };
  const chooseNow = (key: ReminderChoice["key"]) => {
    const fresh = reminderChoiceNow(key, zone, due);
    if (fresh) choose(fresh.write);
    else closeMenu();
  };
  return (
    <>
      {task.remind_at && (
        <p className="t-caption">
          {t("tasks.remind.current", {
            when: formatDue(
              task.remind_at,
              true,
              locale,
              zone,
              task.remind_intent,
            ),
          })}
        </p>
      )}
      {choices.map((choice) => (
        <Button
          key={choice.key}
          disabled={update.isPending}
          onClick={() => chooseNow(choice.key)}
        >
          {t(`tasks.remind.${choice.key}`)}
        </Button>
      ))}
      <Button disabled={update.isPending} onClick={() => setPicking(true)}>
        {t("tasks.remind.pick")}
      </Button>
      {task.remind_at && (
        <Button disabled={update.isPending} onClick={() => choose(null)}>
          {t("tasks.remind.remove")}
        </Button>
      )}
      <ReminderTimeDialog
        open={picking}
        onClose={() => setPicking(false)}
        onPick={async (write) => {
          if (await remind(write)) {
            setPicking(false);
            closeMenu();
          }
        }}
      />
    </>
  );
}

// One write per choice, told in a toast either way. The menu that started it
// has closed, so nothing else on screen says whether it landed.
function useReminderWrite(
  task: ReminderTask,
  update: ReturnType<typeof useTaskUpdate>,
) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const toast = useToast();
  return async (write: ReminderWrite | null): Promise<boolean> => {
    try {
      await update.mutateAsync({
        id: task.id,
        version: task.version,
        body: write ?? { remind_at: null },
      });
    } catch (error) {
      toast.show(problemMessageOf(error, t, t("tasks.remind.failed")), {
        tone: "danger",
      });
      return false;
    }
    toast.show(
      write
        ? t("tasks.remind.set", {
            when: formatDue(
              write.remind_at,
              true,
              locale,
              zone,
              write.remind_intent,
            ),
          })
        : t("tasks.remind.removed"),
    );
    return true;
  };
}

/** A day and a time of day on the reader's clock, which must still be ahead. */
function ReminderTimeDialog({
  open,
  onClose,
  onPick,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  onPick: (write: ReminderWrite) => Promise<void>;
}>) {
  const t = useT();
  const zone = useViewerZone();
  const formId = useId();
  const today = calendarDay(new Date(useNow(MINUTE_MS)), zone);
  const [day, setDay] = useState("");
  const [time, setTime] = useState("");
  const [fault, setFault] = useState<string | undefined>();
  const [saving, setSaving] = useState(false);
  // Each opening starts blank, so a pick abandoned earlier is not offered again.
  const [openedOn, setOpenedOn] = useState(false);
  if (open !== openedOn) {
    setOpenedOn(open);
    if (open) {
      setDay("");
      setTime("");
      setFault(undefined);
    }
  }
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (saving) return;
    if (day === "" || time === "") {
      setFault(t("tasks.remind.needsTime"));
      return;
    }
    const write = reminderAtWallClock(day, time, zone);
    if (Date.parse(write.remind_at) <= Date.now()) {
      setFault(t("tasks.remind.past"));
      return;
    }
    setFault(undefined);
    setSaving(true);
    await onPick(write);
    setSaving(false);
  };
  return (
    <RecordFormDialog
      open={open}
      title={t("tasks.remind.pickTitle")}
      onClose={onClose}
      fieldCount={2}
      unsaved={day !== "" || time !== ""}
      closeDisabled={saving}
      form={
        <form
          id={formId}
          className="form-stack"
          noValidate
          onSubmit={(event) => void submit(event)}
        >
          <Field label={t("tasks.remind.day")} error={fault}>
            {(control) => (
              <DateInput
                {...control}
                value={isoOrBlank(day)}
                min={today}
                max={dueDayBounds(zone).max}
                onChange={(event) => setDay(event.target.value)}
              />
            )}
          </Field>
          <TaskTimeField
            day={day}
            value={time}
            label={t("tasks.remind.time")}
            onChange={setTime}
          />
        </form>
      }
      actions={
        <>
          <Button type="button" onClick={onClose} disabled={saving}>
            {t("create.cancel")}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            pending={saving}
            busyLabel={t("common.saving")}
          >
            {t("tasks.remind.save")}
          </Button>
        </>
      }
    />
  );
}
