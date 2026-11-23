// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { TaskReminderLines } from "../../components/taskreminder";
import { useTaskUpdate } from "../../components/taskwrite";
import { Button, PendingBody } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { useMe } from "../../lib/session";
import type { WorklistItem } from "./worklist.queries";
import { worklistKey } from "./worklist.queries";

/**
 * "Remind me" in a task row's menu, opening the task detail's reminder lines
 * in place. The row does not carry the reminder, so the task is read once the
 * line opens, and its version pins the write.
 */
export function RowTaskReminder({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const me = useMe().data?.user?.id;
  const [open, setOpen] = useState(false);
  // The server reminds the assignee alone, whom a task row names as its owner.
  if (me === undefined || item.owner?.kind !== "user" || item.owner.id !== me)
    return null;
  return (
    <>
      <Button aria-expanded={open} onClick={() => setOpen((was) => !was)}>
        {t("tasks.remind.menu")}
      </Button>
      {open && <RowReminderLines taskId={item.id} />}
    </>
  );
}

function RowReminderLines({ taskId }: Readonly<{ taskId: string }>) {
  const t = useT();
  const task = useActivity(taskId);
  const update = useTaskUpdate([worklistKey]);
  if (task.data) return <TaskReminderLines task={task.data} update={update} />;
  return (
    <>
      {task.isPending && <PendingBody label={t("tasks.detailLoading")} />}
      <ErrorLine error={task.error} />
    </>
  );
}
