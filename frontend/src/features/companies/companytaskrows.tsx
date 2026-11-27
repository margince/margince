// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, UserRound } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { MayBeDone } from "../../components/maybedone";
import { TaskStandingMark } from "../../components/opentaskrow";
import { RecordLinkChips } from "../../components/recordlinks";
import { useTaskHolder } from "../../components/taskassignee";
import { TaskTypeChip } from "../../components/tasktype";
import { Badge } from "../../design-system/atoms";
import { PanelBody, PanelRow } from "../../design-system/panel";
import { Chip } from "../../design-system/readings";
import { sectionState } from "../../design-system/surfacestate";
import { taskDueZone } from "../../format/calendarday";
import { formatDueCompact } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { taskWriteKeys } from "../../lib/activitykeys";
import { useCompanyZone } from "../../lib/companyzone";
import { isTaskType } from "../../lib/tasktype";
import { useViewerZone } from "../../lib/viewerzone";
import "../../components/taskmeta.css";
import "./companytabs.css";

type Company360 = components["schemas"]["Company360"];
type NextStep = NonNullable<Company360["next_steps"]>["data"][number];
type ActivityLink = components["schemas"]["ActivityLink"];

type RowVerbs = Readonly<{
  onOpenTask?: (step: NextStep) => void;
  // The completion tick before the task's name. Absent (a read-only account)
  // draws none: a box that cannot be ticked is worse than no box.
  renderTick?: (step: NextStep) => ReactNode;
  renderAction?: (step: NextStep) => ReactNode;
}>;

/**
 * The account's open tasks as rows of the Tasks list, each shaped like a
 * Worklist row. A row holds the tick, the task, its own words and the chip
 * line. A withheld section is the caller's to word.
 */
export function CompanyTaskRows({
  view,
  ...verbs
}: Readonly<{ view: Company360 } & RowVerbs>) {
  const t = useT();
  const steps = view.next_steps?.data ?? [];
  const state = sectionState(
    view,
    "next_steps",
    Boolean(view.next_steps),
    steps.length,
  );
  if (state === "empty") {
    return (
      <PanelBody>
        <p className="co-tasklist-none">{t("co.next.empty")}</p>
      </PanelBody>
    );
  }
  if (state === "unavailable") {
    return (
      <PanelBody>
        <p className="surfacestate-withheld">{t("co.section.unavailable")}</p>
      </PanelBody>
    );
  }
  return steps.map((step) => (
    <CompanyTaskRow
      key={step.activity_id}
      step={step}
      companyId={view.company.id}
      asOf={view.as_of}
      {...verbs}
    />
  ));
}

function CompanyTaskRow({
  step,
  companyId,
  asOf,
  onOpenTask,
  renderTick,
  renderAction,
}: Readonly<{ step: NextStep; companyId: string; asOf: string } & RowVerbs>) {
  const why = leadParagraph(step.body);
  return (
    <PanelRow className="co-taskrow">
      <span className="co-taskrow-tick">{renderTick?.(step)}</span>
      <div className="co-taskrow-text">
        {onOpenTask ? (
          <button
            type="button"
            className="co-taskrow-title"
            onClick={() => onOpenTask(step)}
          >
            {step.subject}
          </button>
        ) : (
          <span className="co-taskrow-title">{step.subject}</span>
        )}
        {why && <p className="t-caption co-taskrow-why">{why}</p>}
        <TaskRowMeta step={step} asOf={asOf} />
        {/* A list that offers the tick asks the Worklist's question too. */}
        {renderTick && (
          <MayBeDone
            question={step.may_be_done}
            task={{
              id: step.activity_id,
              version: step.version,
              canComplete: true,
            }}
            refresh={[...taskWriteKeys("company", companyId), ["worklist"]]}
          />
        )}
      </div>
      {renderAction && (
        <span className="co-taskrow-acts">{renderAction(step)}</span>
      )}
    </PanelRow>
  );
}

// The chip line Home's cards draw: the due day and how it stands, the type,
// the records, and who holds it.
function TaskRowMeta({
  step,
  asOf,
}: Readonly<{ step: NextStep; asOf: string }>) {
  const { locale } = useLocale();
  // A date-only task falls on the company's calendar, as the Worklist groups it.
  const zone = taskDueZone(
    step.due_has_time,
    useViewerZone(),
    useCompanyZone(),
  );
  const holder = useTaskHolder(step.assignee_id, step.assignee_name);
  const links: ActivityLink[] = [];
  if (step.linked_contact_id) {
    links.push({ entity_type: "contact", entity_id: step.linked_contact_id });
  }
  if (step.linked_deal_id) {
    links.push({ entity_type: "deal", entity_id: step.linked_deal_id });
  }
  return (
    <span className="task-meta">
      {step.due_at && (
        <>
          <Badge icon={CalendarDays}>
            {formatDueCompact(
              step.due_at,
              step.due_has_time,
              locale,
              zone,
              step.due_intent,
            )}
          </Badge>
          <TaskStandingMark task={step} asOf={asOf} />
        </>
      )}
      {step.task_type && isTaskType(step.task_type) && (
        <TaskTypeChip type={step.task_type} />
      )}
      <RecordLinkChips links={links} />
      {holder !== null && (
        <Chip dense icon={UserRound}>
          {holder}
        </Chip>
      )}
    </span>
  );
}

// The task's own words up to their first blank line: the sentence that says
// why it is on the list. The rest is the details' to show.
function leadParagraph(body: string | null | undefined): string {
  return (body ?? "").trim().split(/\n\s*\n/)[0] ?? "";
}
