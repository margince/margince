// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A lead contributes recorded tasks; its existence does not create a reply
// obligation. Incoming requests appear through the waiting-message queue.

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { formatDateAbbrev } from "../format/format";
import { daysPast } from "../format/lateness";
import type { Locale, Translator } from "../i18n";
import { TodoRow } from "./record360";

type Lead = components["schemas"]["Lead"];

// A closed lead is not worked and draws no rows at all.
export function leadTodoRows(
  lead: Lead,
  t: Translator,
  locale: Locale,
  recordZone: string,
  // The same queue the panel head's own way out opens: the lead names no
  // task id of its own, so the queue is the honest destination for "the next
  // one" too.
  onOpenTasks: () => void,
): ReactNode[] {
  if (lead.archived_at) {
    return [];
  }
  const rows: ReactNode[] = [];
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
