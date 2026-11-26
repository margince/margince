// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays } from "lucide-react";
import { useState } from "react";
import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { ChipFace, chipClass } from "../design-system/chipface";
import { Popover } from "../design-system/popover";
import { provenanceOf } from "../design-system/provenanceof";
import { Chip } from "../design-system/readings";
import { formatDue } from "../format/format";
import { useLocale, useT } from "../i18n";
import { taskTypeOf } from "../lib/tasktype";
import { useViewerZone } from "../lib/viewerzone";
import { mostSpecificFirst, RecordLinkChips } from "./recordlinks";
import { TaskDueDatePick } from "./taskduedatepick";
import { TaskQuickActions } from "./taskquickactions";
import { TaskRemindMenu } from "./taskreminder";
import { TaskTypeChip } from "./tasktype";
import type { useTaskUpdate } from "./taskwrite";

type Activity = components["schemas"]["Activity"];

/**
 * The task's facts as one line of chips under its title, as on Home's cards.
 * They say when it is due, its type and the records it is with. Where the
 * reader may act, the due chip opens Move and Snooze, and a last chip sets the
 * reminder.
 */
export function TaskDetailMeta({
  task,
  update,
  canAct,
}: Readonly<{
  task: Activity;
  update: ReturnType<typeof useTaskUpdate>;
  canAct: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  // The record's own clock: a deadline is a fact colleagues read back, so its
  // day cannot depend on where the reader sits.
  const due = task.due_at
    ? t("co.next.due", {
        when: formatDue(
          task.due_at,
          task.due_has_time,
          locale,
          readerZone,
          task.due_intent,
        ),
      })
    : t("co.next.undated");
  return (
    <div className="task-detail-meta">
      {canAct ? (
        <TaskDuePopover task={task} update={update} label={due} />
      ) : (
        <Chip dense icon={CalendarDays}>
          {due}
        </Chip>
      )}
      <TaskTypeChip type={taskTypeOf(task)} />
      {task.links && task.links.length > 0 && (
        <RecordLinkChips links={mostSpecificFirst(task.links)} />
      )}
      {canAct && <TaskRemindMenu task={task} update={update} />}
      {task.is_done && <Badge tone="success">{t("tasks.isDone")}</Badge>}
      {/* A task an agent wrote says so, so a reader knows it was read out of
          a conversation rather than typed by a colleague. */}
      {provenanceOf(task.captured_by).kind === "agent" && (
        <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
      )}
    </div>
  );
}

/** The due chip, which opens the day and time to move the task to. */
function TaskDuePopover({
  task,
  update,
  label,
}: Readonly<{
  task: Activity;
  update: ReturnType<typeof useTaskUpdate>;
  label: string;
}>) {
  const [open, setOpen] = useState(false);
  return (
    <Popover
      label={
        <ChipFace mark={<CalendarDays size={12} aria-hidden="true" />}>
          {label}
        </ChipFace>
      }
      className={`${chipClass({ "chip-dense": true })} chip-link`}
      open={open}
      onOpenChange={setOpen}
      dialog
      width="wide"
    >
      <div className="task-detail-move">
        {task.due_at && (
          <div className="task-detail-buttons">
            <TaskQuickActions
              showComplete={false}
              activityId={task.id}
              version={task.version}
              dueAt={task.due_at}
              dueHasTime={task.due_has_time}
              dueIntent={task.due_intent}
              update={update}
            />
          </div>
        )}
        <TaskDueDatePick
          activityId={task.id}
          version={task.version}
          dueAt={task.due_at}
          dueHasTime={task.due_has_time}
          dueIntent={task.due_intent}
          update={update}
          onMoved={() => setOpen(false)}
        />
      </div>
    </Popover>
  );
}
