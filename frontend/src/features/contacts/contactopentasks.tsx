// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { type OpenTask, OpenTaskRow } from "../../components/opentaskrow";
import { useTaskUpdate } from "../../components/taskactions";
import { Button } from "../../design-system/atoms";
import { OverviewCard, OverviewList } from "../../design-system/overviewcard";
import { daysPast } from "../../format/lateness";
import { useT } from "../../i18n";
import { displayedSubject } from "../../lib/systemdisplay";
import { worklistKey } from "../../lib/workkeys";
import { omitted } from "./contact360";
import { nextBarChoice } from "./contacttoday";

// The contact's open tasks under the Next bar, soonest due first as the 360
// sends them, each with its Done tick. The task the bar draws is not listed
// again.

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];

export function ContactOpenTasks({
  view,
  onAllTasks,
  onAddTask,
}: Readonly<{
  view: Contact360;
  onAllTasks: () => void;
  // Opens the task form on this contact; absent where the reader may not log.
  onAddTask?: () => void;
}>) {
  const t = useT();
  const contactId = view.contact.id;
  const taskUpdate = useTaskUpdate([["contact360", contactId], worklistKey]);
  const withheld = omitted(view, "next_steps");
  const choice = nextBarChoice(view.moment, view);
  const rows = withheld ? [] : choice.rest;
  // The bar may hold the one open task, and then nothing else is.
  const barHoldsOne =
    choice.task !== undefined || choice.rest.length < openTaskCount(view);
  return (
    <OverviewCard
      title={t("contact.card.openTasks")}
      action={
        <Button variant="link" onClick={onAllTasks}>
          {t("record.allTasks")}
        </Button>
      }
      empty={
        withheld
          ? t("state.withheld")
          : t(
              barHoldsOne
                ? "co.overview.nothingOpen"
                : "contact.card.noOpenTasks",
            )
      }
      emptyAction={
        !withheld &&
        onAddTask && (
          <Button variant="ghost" onClick={onAddTask}>
            {t("contact.action.addTask")}
          </Button>
        )
      }
    >
      {rows.length > 0 && (
        <OverviewList>
          {rows.map((task) => (
            <OpenTaskRow
              key={task.id}
              task={openTaskOf(task, view.as_of, t("task.untitled"))}
              asOf={view.as_of}
              taskUpdate={taskUpdate}
            />
          ))}
        </OverviewList>
      )}
    </OverviewCard>
  );
}

function openTaskCount(view: Contact360): number {
  return (view.next_steps?.data ?? []).filter((task) => !task.is_done).length;
}

// A contact task as an Overview row reads it. Late by the same measure as the
// "+N tasks" peek beside it.
function openTaskOf(task: Activity, asOf: string, untitled: string): OpenTask {
  return {
    id: task.id,
    subject: displayedSubject(task) ?? untitled,
    due_at: task.due_at,
    due_has_time: task.due_has_time,
    overdue: task.due_at
      ? daysPast(Date.parse(task.due_at), Date.parse(asOf)).late
      : false,
    assignee_id: task.assignee_id,
    version: task.version,
    writable: task.writable === true,
  };
}
