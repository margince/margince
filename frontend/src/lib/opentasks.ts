// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components, operations } from "../api/schema";
import { unwrap } from "./problem";

type Activity = components["schemas"]["Activity"];
type TaskRecordType = NonNullable<
  NonNullable<
    operations["listActivities"]["parameters"]["query"]
  >["entity_type"]
>;

// The page size the list read allows at most; a record past it is read page by page.
const TASK_PAGE = 200;

// Checked on the row too, not only asked of the server. A row that is not a
// task carries no is_done, so it would read as an open task.
function isOpenTask(row: Activity): boolean {
  return row.kind === "task" && row.is_done !== true;
}

/**
 * The open tasks filed on one record. Under the record's timeline key, so a
 * task written anywhere on it refreshes the list.
 */
export function openTasksQuery(recordType: TaskRecordType, recordId: string) {
  return {
    queryKey: ["activities", recordType, recordId, "open-tasks"],
    queryFn: () => readOpenTasks(recordType, recordId),
  };
}

async function readOpenTasks(
  recordType: TaskRecordType,
  recordId: string,
): Promise<Activity[]> {
  const open: Activity[] = [];
  let cursor: string | undefined;
  do {
    const page = unwrap(
      await api.GET("/activities", {
        params: {
          query: {
            entity_type: recordType,
            entity_id: recordId,
            kind: ["task"],
            limit: TASK_PAGE,
            cursor,
          },
        },
      }),
    );
    open.push(...page.data.filter(isOpenTask));
    cursor = page.page.has_more
      ? (page.page.next_cursor ?? undefined)
      : undefined;
  } while (cursor);
  return open;
}

export function useOpenTasks(
  recordType: TaskRecordType,
  recordId: string | null,
) {
  return useQuery({
    ...openTasksQuery(recordType, recordId ?? ""),
    enabled: recordId !== null,
  });
}
