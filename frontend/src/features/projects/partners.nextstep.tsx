// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { LogActivityAction } from "../../components/logactivity.action";
import { OpenTaskRow, openTaskOf } from "../../components/opentaskrow";
import { useTaskUpdate } from "../../components/taskactions";
import { ErrorLine } from "../../design-system/errorline";
import { OverviewList } from "../../design-system/overviewcard";
import { formatDate } from "../../format/format";
import { useNow } from "../../format/now";
import { useLocale, useT } from "../../i18n";
import { taskWriteKeys } from "../../lib/activitykeys";
import { useCanWrite } from "../../lib/capability";
import { useOpenTasks } from "../../lib/opentasks";
import { useViewerZone } from "../../lib/viewerzone";
import type { Partner } from "./partners.vocab";

type Activity = components["schemas"]["Activity"];

/**
 * The partner's next step is a task on its company, with a colleague who does
 * it and a due date. So it reaches that colleague's Home and Worklist. Text
 * written before that stays readable here until someone turns it into a task.
 */
export function PartnerNextStep({
  companyId,
  partner,
}: Readonly<{ companyId: string; partner: Partner }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const canFileTask = useCanWrite("activity", "create");
  const tasks = useOpenTasks("company", companyId);
  const taskUpdate = useTaskUpdate(taskWriteKeys("company", companyId));
  // Live, so a task that passes its deadline while the page is open turns late.
  const asOf = new Date(useNow(60_000)).toISOString();
  const due = partner.next_step_due_at;
  return (
    <div className="form-stack">
      {(partner.next_step || due) && (
        <p>
          {partner.next_step && <span>{partner.next_step}</span>}
          {partner.next_step && due && " · "}
          {due && <span>{formatDate(due, locale, zone)}</span>}
          <br />
          <span className="t-caption">{t("partner.nextStepOld")}</span>
        </p>
      )}
      <ErrorLine error={tasks.error} />
      {tasks.data && tasks.data.length > 0 && (
        <OverviewList>
          {soonestFirst(tasks.data).map((task) => (
            <OpenTaskRow
              key={task.id}
              task={openTaskOf(task, asOf, t("task.untitled"))}
              asOf={asOf}
              taskUpdate={taskUpdate}
            />
          ))}
        </OverviewList>
      )}
      {canFileTask && (
        <LogActivityAction
          entityType="company"
          entityId={companyId}
          askedKind="task"
        />
      )}
    </div>
  );
}

// Soonest due first, and a task with no date after every dated one.
function soonestFirst(tasks: readonly Activity[]): Activity[] {
  const dueOf = (task: Activity) =>
    task.due_at ? Date.parse(task.due_at) : Number.POSITIVE_INFINITY;
  return [...tasks].sort((a, b) => dueOf(a) - dueOf(b));
}
