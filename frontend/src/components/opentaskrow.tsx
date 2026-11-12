// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge } from "../design-system/atoms";
import { OverviewRow } from "../design-system/overviewcard";
import { formatNumber } from "../format/format";
import { formatDayMonth, formatDueCompact } from "../format/format.dates";
import { useLocale, usePlural, useT } from "../i18n";
import { type DatedTask, taskStanding } from "../lib/taskstanding";
import { useViewerZone } from "../lib/viewerzone";
import { TaskCompleteCheck, type useTaskUpdate } from "./taskactions";
import { taskLine, useTaskHolder } from "./taskassignee";

// One open task as a row of a record's Overview card: the tick, who holds it,
// when it is due, and where it stands. The company and contact pages both
// draw their open tasks with it.

type TaskUpdate = ReturnType<typeof useTaskUpdate>;

/** The fields a row reads, whichever record's read the task came from. */
export type OpenTask = DatedTask &
  Readonly<{
    id: string;
    subject: string;
    due_has_time?: boolean;
    assignee_id?: string | null;
    assignee_name?: string | null;
    version?: number;
    // Whether this reader may complete it. A box that cannot be ticked is
    // worse than none, so a task they may not change draws no tick.
    writable: boolean;
  }>;

export function OpenTaskRow({
  task,
  asOf,
  taskUpdate,
  title,
  forName,
}: Readonly<{
  task: OpenTask;
  asOf: string;
  // The tick's write. Absent draws no tick.
  taskUpdate?: TaskUpdate;
  // The title as the caller draws it, a link to the task for one; the subject
  // as text when absent.
  title?: ReactNode;
  // The record the task is for, where the card spans more than one.
  forName?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const holder = useTaskHolder(task.assignee_id, task.assignee_name);
  const sub = taskLine([
    holder,
    task.due_at &&
      t("co.next.due", {
        when: formatDueCompact(task.due_at, task.due_has_time, locale, zone),
      }),
    forName && t("co.overview.for", { name: forName }),
  ]);
  return (
    <OverviewRow
      lead={
        taskUpdate &&
        task.writable && (
          <TaskCompleteCheck
            activityId={task.id}
            version={task.version}
            update={taskUpdate}
          />
        )
      }
      title={title ?? task.subject}
      sub={sub}
      right={<TaskStandingMark task={task} asOf={asOf} />}
    />
  );
}

/** Where a task stands, as the right end of its row: a pill while it is late
 *  or close, the day itself further out, "No date" when it has none. */
export function TaskStandingMark({
  task,
  asOf,
}: Readonly<{ task: DatedTask; asOf: string }>): ReactNode {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const standing = taskStanding(task, asOf, zone);
  switch (standing.kind) {
    case "late":
      return (
        <Badge tone="danger">
          {standing.days === undefined
            ? t("co.next.overdue")
            : plural("co.next.daysLate", standing.days, {
                days: formatNumber(standing.days, locale),
              })}
        </Badge>
      );
    case "today":
      return <Badge tone="warning">{t("co.next.dueToday")}</Badge>;
    case "soon":
      return (
        <Badge tone="warning">
          {plural("co.next.inDays", standing.days, {
            days: formatNumber(standing.days, locale),
          })}
        </Badge>
      );
    case "later":
      return formatDayMonth(standing.due, locale, zone);
    case "undated":
      return t("co.overview.noDate");
  }
}
