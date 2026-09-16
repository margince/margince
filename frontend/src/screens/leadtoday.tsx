// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHAT NEEDS A CONTACT ON THIS LEAD: the first response, while it is owed,
// and the next task on it, each drawn as a to-do the record already carries
// with the verb that resolves it. Neither is the agent's move: a lead
// carries no suggestions. Split out of leads.tsx so the rows can be tested
// on their own, the way company360 and contact360 already keep theirs in
// companytoday.tsx and contacttoday.tsx.

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { formatDateAbbrev, formatDateTime } from "../format/format";
import { daysPast } from "../format/lateness";
import { leadIdentityName } from "../format/leadname";
import { viewerZone } from "../format/timezone";
import type { Locale, Translator } from "../i18n";
import { TodoRow } from "./record360";

type Lead = components["schemas"]["Lead"];

// A closed lead is not worked and draws no rows at all.
export function leadTodoRows(
  lead: Lead,
  t: Translator,
  locale: Locale,
  // A task's DEADLINE is the record's day: the team agreed it and every
  // colleague must quote the same one. The response clocks below stay on the
  // reader's own: how long a lead has been waiting on them is about them.
  recordZone: string,
  // Brings the page's own composer into view and hands it focus: the row
  // sends the reader to the one place this page already writes a reply,
  // rather than opening a second one of its own.
  onReply: () => void,
  // The same queue the panel head's "View tasks" opens: the lead names no
  // task id of its own, so the queue is the honest destination for "the next
  // one" too.
  onOpenTasks: () => void,
  // Set while the page's write is refused for a reason other than being
  // closed (a closed lead draws no rows at all), so the Reply verb points at
  // the same sentence the header's Email verb does, rather than a press that
  // does nothing.
  replyReasonId: string | undefined,
): ReactNode[] {
  if (lead.archived_at) {
    return [];
  }
  const zone = viewerZone();
  const rows: ReactNode[] = [];
  if (!lead.first_response_at) {
    rows.push(
      <TodoRow
        key="answer"
        title={t("lead.today.answer", {
          name: leadIdentityName(lead) || t("lead.unnamed"),
        })}
        meta={t("lead.today.answerMeta")}
        due={firstResponseDue(lead, t, locale, zone)}
        action={
          <Button
            small
            variant="ghost"
            reasonId={replyReasonId}
            onClick={onReply}
          >
            {t("lead.today.reply")}
          </Button>
        }
      />,
    );
  }
  if (lead.next_task_subject) {
    const late = lead.next_task_due_at
      ? daysPast(Date.parse(lead.next_task_due_at), Date.now()).late
      : false;
    rows.push(
      <TodoRow
        key="task"
        title={lead.next_task_subject}
        meta={t("lead.today.nextTask")}
        due={
          lead.next_task_due_at
            ? {
                label: late
                  ? t("co.next.overdue")
                  : t("co.next.due", {
                      when: formatDateAbbrev(
                        lead.next_task_due_at,
                        locale,
                        recordZone,
                      ),
                    }),
                tone: late ? "danger" : undefined,
              }
            : { label: t("co.next.undated") }
        }
        verb={{ label: t("lead.today.openTasks"), onAct: onOpenTasks }}
      />,
    );
  }
  return rows;
}

// When the first response is owed, in the server's own three states. No clock
// means owed without a date, which is not the same as late.
function firstResponseDue(
  lead: Lead,
  t: Translator,
  locale: Locale,
  zone: string,
): { label: string; tone?: "warn" | "danger" } | undefined {
  if (!lead.sla_deadline_at || !lead.sla_state) {
    return undefined;
  }
  if (lead.sla_state === "breached") {
    return { label: t("co.next.overdue"), tone: "danger" };
  }
  return {
    label: t("co.next.due", {
      when: formatDateTime(lead.sla_deadline_at, locale, zone),
    }),
    tone: lead.sla_state === "at_risk" ? "warn" : undefined,
  };
}
