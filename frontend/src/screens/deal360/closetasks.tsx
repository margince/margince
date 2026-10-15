// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's open tasks, asked about at the moment the deal closes.
//
// Closing a deal keeps its tasks open, because a task on a closed deal can
// still be real work. So the close asks, and completes them only when the
// reader picks Complete all, through the bulk change and its usual undo.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  type BulkBatch,
  type BulkChangeRequest,
  useBulkOutcome,
} from "../../components/bulkchange";
import {
  BULK_MAX_ITEMS,
  executeBulkChange,
  previewBulkChange,
} from "../../components/bulkchangeapi";
import { SkipReason } from "../../components/bulkwords";
import { ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";

type Activity = components["schemas"]["Activity"];

export type CloseTasksAnswer = "complete" | "keep";

// The page size the list read allows at most; a deal past it is read page by page.
const TASK_PAGE = 200;

// Checked on the row too, not only asked of the server. A row that is not a
// task carries no is_done, so it would read as an open task.
function isOpenTask(row: Activity): boolean {
  return row.kind === "task" && row.is_done !== true;
}

/**
 * The open tasks filed on the deal itself. A task filed only on the deal's
 * company or contact is not the deal's, so the deal-link narrowing leaves it
 * out. Under the deal's timeline key, so a task written anywhere refreshes it.
 */
function useDealOpenTasks(dealId: string | null) {
  return useQuery({
    queryKey: ["activities", "deal", dealId, "open-tasks"],
    enabled: dealId !== null,
    queryFn: async () => {
      const open: Activity[] = [];
      let cursor: string | undefined;
      do {
        const page = unwrap(
          await api.GET("/activities", {
            params: {
              query: {
                entity_type: "deal",
                entity_id: dealId ?? "",
                kind: "task",
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
    },
  });
}

type BulkSkip = components["schemas"]["BulkSkip"];

// The listed tasks as bulk changes of at most the contract's bound each.
function completionRequests(tasks: readonly Activity[]): BulkChangeRequest[] {
  const requests: BulkChangeRequest[] = [];
  for (let at = 0; at < tasks.length; at += BULK_MAX_ITEMS) {
    requests.push({
      recordType: "worklist_item",
      verb: "complete",
      rows: tasks.slice(at, at + BULK_MAX_ITEMS).map((task) => ({
        id: task.id,
        version: task.version,
        label: task.subject ?? task.id,
      })),
      openId: crypto.randomUUID(),
    });
  }
  return requests;
}

/**
 * Completes the tasks the dialog listed, in bulk changes of at most
 * BULK_MAX_ITEMS. Each preview mints the confirm token its execute needs above
 * ten records. One note reports them all; its Undo reverses every batch.
 */
function useCompleteDealTasks() {
  const t = useT();
  const toast = useToast();
  const report = useBulkOutcome();
  return async (tasks: readonly Activity[]) => {
    const batches: BulkBatch[] = [];
    let firstSkip: BulkSkip | undefined;
    let failed = false;
    try {
      for (const request of completionRequests(tasks)) {
        const preview = await previewBulkChange(request, t);
        firstSkip ??= preview.excluded[0];
        if (preview.count === 0) {
          continue;
        }
        const result = await executeBulkChange({
          request,
          confirmToken: preview.confirm_token,
          idempotencyKey: request.openId,
          t,
        });
        batches.push({ request, result });
      }
    } catch {
      failed = true;
    }
    // Batches that ran before a failure are reported too, so their Undo stays.
    if (batches.some((batch) => batch.result.changed > 0)) {
      await report(batches);
    } else if (!failed && tasks.length > 0) {
      toast.show(<NothingCompleted skip={firstSkip} />, { tone: "warning" });
    }
    if (failed) {
      // Said here because the dialog that asked is gone by now.
      toast.show(t("deals.closeTasks.failed"), { tone: "danger" });
    }
  };
}

// Why Complete all changed nothing, with the server's reason when it gave one.
function NothingCompleted({ skip }: Readonly<{ skip: BulkSkip | undefined }>) {
  const t = useT();
  return (
    <>
      {t("deals.closeTasks.noneCompleted")}
      {skip && (
        <>
          {" "}
          <SkipReason skip={skip} verb="complete" />
        </>
      )}
    </>
  );
}

/**
 * The question as the confirm dialog holds it: the tasks listed, the reader's
 * answer, and the tasks that answer completes once the close lands.
 *
 * Confirm waits while any page of the read is pending, so a fast press cannot
 * skip the question. A failed read does not block the close; it keeps every
 * task open and says so.
 */
export function useCloseTasks(dealId: string | null) {
  const t = useT();
  const [answer, setAnswer] = useState<CloseTasksAnswer | "">("");
  const read = useDealOpenTasks(dealId);
  const listed = read.data ?? [];
  const complete = useCompleteDealTasks();
  return {
    listed,
    answer,
    setAnswer,
    readError: read.error,
    /** Why Confirm waits: the read is still out, or listed tasks lack an answer. */
    refusal: closeRefusal(dealId !== null && read.isPending, listed, answer, t),
    /** What Complete all completes: the tasks the reader saw, not a fresh read. */
    chosen: answer === "complete" ? listed : [],
    complete,
    reset: () => setAnswer(""),
  };
}

function closeRefusal(
  reading: boolean,
  listed: readonly Activity[],
  answer: CloseTasksAnswer | "",
  t: ReturnType<typeof useT>,
): string | undefined {
  if (reading) {
    return t("deals.closeTasks.loading");
  }
  return listed.length > 0 && answer === ""
    ? t("deals.closeTasks.choose")
    : undefined;
}

/** The question itself: the deal's open tasks, and Complete all or Keep. */
export function CloseTasksQuestion({
  state,
}: Readonly<{ state: ReturnType<typeof useCloseTasks> }>) {
  const { listed: tasks, answer, setAnswer: onAnswer, readError } = state;
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (readError) {
    return (
      <>
        <ErrorLine error={readError} />
        <p className="t-caption">{t("deals.closeTasks.readFailed")}</p>
      </>
    );
  }
  if (tasks.length === 0) {
    return null;
  }
  const count = { count: formatNumber(tasks.length, locale) };
  return (
    <>
      <ul className="t-caption" aria-label={t("deals.closeTasks.listLabel")}>
        {tasks.map((task) => (
          <li key={task.id}>
            {task.subject ?? t("deals.closeTasks.untitled")}
          </li>
        ))}
      </ul>
      <ChoiceList<CloseTasksAnswer>
        legend={plural("deals.closeTasks.legend", tasks.length, count)}
        value={answer}
        onChange={onAnswer}
        choices={[
          {
            value: "complete",
            label: t("deals.closeTasks.complete"),
            description: t("deals.closeTasks.completeHint"),
          },
          {
            value: "keep",
            label: t("deals.closeTasks.keep"),
            description: t("deals.closeTasks.keepHint"),
          },
        ]}
      />
    </>
  );
}
