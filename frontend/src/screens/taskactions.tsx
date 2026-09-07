import {
  type QueryKey,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useRecordZone } from "../app/recordzone";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  Modal,
  PendingBody,
} from "../design-system/atoms";
import { DateInput, isISODate } from "../design-system/dateinput";
import { calendarDay, dueInstant } from "../format/calendarday";
import { formatDate, formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { EntityRef } from "./entityref";

// Acting on a task from the record it belongs to. The tasks screen owns the
// standing work queue; this is the same two verbs (complete, snooze) offered
// where the rep already is, plus the detail a next-step row has no room for.
//
// Which cached reads a task write invalidates depends on where it was written
// from, so the caller passes them in — the work queue is workspace-wide, the
// record's timeline is not.

type Activity = components["schemas"]["Activity"];
type TaskPatch = {
  id: string;
  // The version the press was decided against. Every verb on a task goes
  // through this one mutation, so pinning it here pins all four at once — and
  // an unpinned tick is a task two people can complete, each told it worked.
  version: number | undefined;
  body: { is_done?: boolean; due_at?: string; remind_at?: string | null };
};

const ONE_DAY_MS = 86_400_000;

export function useTaskUpdate(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: TaskPatch) => {
      const { data, error } = await api.PATCH("/activities/{id}", {
        params: {
          path: { id: input.id },
          ...ifMatch(requireVersion(input.version)),
        },
        body: input.body,
      });
      if (error) {
        throwProblem(error, t);
      }
      // The version the write PRODUCED, answered so a follow-on press has one.
      // An undo re-sending the version the row was drawn at would be refused as
      // skew by the very write it is undoing.
      return data?.version;
    },
    onSuccess: (_data, input) => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
      // The task's own detail read too, always: a modal open on the task that
      // was just completed would otherwise keep showing the old due date and
      // offering the verbs that no longer apply.
      queryClient.invalidateQueries({ queryKey: ["activity", input.id] });
    },
  });
}

/** The next due date one snooze away, or null for a task that has no date to move. */
export function snoozedDueAt(dueAt: string | null | undefined): string | null {
  if (!dueAt) {
    return null;
  }
  return new Date(new Date(dueAt).getTime() + ONE_DAY_MS).toISOString();
}

/**
 * TaskCompleteCheck is the tick affordance itself — a checkbox rather than a
 * labelled button, for a row that names its own verb (ticking IS completing)
 * rather than one that reads as a menu of actions. Every caller lists OPEN
 * tasks only, so the box always starts unchecked; ticking it fires the
 * completion and the row leaves the list on the invalidated re-read, rather
 * than the checkbox itself flipping to a done state it would then have to
 * keep showing.
 */
export function TaskCompleteCheck({
  activityId,
  version,
  update,
}: Readonly<{
  activityId: string;
  version: number | undefined;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const isThisTask = update.variables?.id === activityId;
  const pending = update.isPending && isThisTask;
  // A rejected PATCH re-enables the box and leaves it unchecked — the same
  // rendering a click that did nothing would leave. Without this, the two are
  // indistinguishable and the reader has no reason to try again.
  const failed = update.isError && isThisTask;
  return (
    <>
      {/* The row already names the task beside it, so the tick's own name is
          screen-reader-only — the words are not repeated on screen, and the
          box still announces what ticking it does. */}
      <Checkbox
        label={<span className="sr-only">{t("tasks.complete")}</span>}
        checked={false}
        disabled={pending}
        onChange={() =>
          update.mutate({ id: activityId, version, body: { is_done: true } })
        }
      />
      {failed && (
        <span className="co-part-error" role="alert">
          {problemMessageOf(update.error, t)}
        </span>
      )}
    </>
  );
}

/**
 * TaskQuickActions is the verbs a rep needs on a next-step row beyond the tick.
 *
 * Snooze, offered only for a DATED task, since one day after nothing is
 * nothing. And a date picker, offered always — an undated task is exactly the
 * one a rep wants to put a day on, and it is the one the snooze cannot serve.
 *
 * The two are not the same verb spelled twice. Snooze answers "not yet" in one
 * press and moves the task's own due date by a day; the picker answers
 * "Tuesday", which the snooze cannot reach in principle rather than merely in
 * clicks — a task three days overdue snoozes to two days overdue.
 *
 * Complete lives on `TaskCompleteCheck` instead — `showComplete` keeps it here
 * too for the one caller (the detail modal) that has no row-level checkbox of
 * its own to tick.
 */
export function TaskQuickActions({
  activityId,
  version,
  dueAt,
  update,
  showComplete = true,
  showDuePicker = false,
}: Readonly<{
  activityId: string;
  version: number | undefined;
  dueAt?: string | null;
  update: ReturnType<typeof useTaskUpdate>;
  showComplete?: boolean;
  // Whether the reader may name a day, rather than only step the due date on.
  // OFF by default, and the default is the point: this is a per-row action slot
  // on the 360 next-steps lists, where a date box in every row is heavier than
  // the surface intends. The detail modal opts in, because that is where a
  // reader already came to work on one task.
  showDuePicker?: boolean;
}>) {
  const t = useT();
  const nextDue = snoozedDueAt(dueAt);
  const pending = update.isPending && update.variables?.id === activityId;
  return (
    <>
      {showComplete && (
        <Button
          small
          variant="primary"
          disabled={pending}
          onClick={() =>
            update.mutate({ id: activityId, version, body: { is_done: true } })
          }
        >
          {t("tasks.complete")}
        </Button>
      )}
      {nextDue && (
        <Button
          small
          disabled={pending}
          onClick={() =>
            update.mutate({
              id: activityId,
              version,
              body: { due_at: nextDue },
            })
          }
        >
          {t("tasks.snooze")}
        </Button>
      )}
      {showDuePicker && (
        <TaskDueDatePick
          activityId={activityId}
          version={version}
          dueAt={dueAt}
          update={update}
        />
      )}
    </>
  );
}

// Moving a task to a day the reader names.
//
// Beside the snooze rather than instead of it, because they answer different
// questions. Snooze is "not yet" and takes one press; this is "Tuesday",
// which no number of presses reaches — and the snooze cannot reach it in
// principle, not just in clicks: it adds a day to the task's OWN due date, so
// a task three days overdue snoozes to two days overdue and the reader presses
// it four times to mean tomorrow.
//
// It is offered on an UNDATED task too, where the snooze is not. A task with no
// day has nothing to move but is exactly the one a rep wants to put a date on,
// and the button above is hidden for it — snoozedDueAt returns null, because
// one day after nothing is nothing.
function TaskDueDatePick({
  activityId,
  version,
  dueAt,
  update,
}: Readonly<{
  activityId: string;
  version: number | undefined;
  dueAt?: string | null;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const pending = update.isPending && update.variables?.id === activityId;
  // Seeded from the task's own day in the VIEWER's zone, matching what the
  // meta line above reads it in: a picker opening on a different day than the
  // one displayed beside it would be two answers to "when is this due".
  const seeded = dueAt ? calendarDay(new Date(dueAt), viewerZone()) : "";
  // Narrowed rather than asserted. calendarDay returns a string, and the
  // control's type says it takes a calendar day or nothing — so a value that
  // is neither opens the picker empty instead of feeding the element something
  // it would silently reject.
  const current = isISODate(seeded) ? seeded : "";
  return (
    <Field label={t("tasks.moveTo")}>
      {(field) => (
        <DateInput
          {...field}
          value={current}
          disabled={pending}
          onChange={(event) => {
            const day = event.target.value;
            // Narrowed with the control's own guard, not merely tested for
            // empty. A `type="date"` box clears itself for anything it cannot
            // parse — `2026-02-30` arrives as "" — but HTML permits a year of
            // FOUR OR MORE digits, so `10000-09-15` is a value the element
            // reports happily and `dueInstant` throws a RangeError on. A year
            // typo would have reached an uncaught exception with nothing on
            // screen to say the date was refused.
            //
            // isISODate requires exactly four, which refuses that and the
            // cleared box in one question — and clearing IS a case to refuse:
            // it is the browser's own empty state, not a request to undate the
            // task.
            if (!isISODate(day)) {
              return;
            }
            update.mutate({
              id: activityId,
              version,
              body: { due_at: dueInstant(day) },
            });
          }}
        />
      )}
    </Field>
  );
}

/**
 * TaskDetailModal opens one task where it was listed.
 *
 * A task has no screen of its own — it lives in a timeline, not on a record
 * page — so the detail comes to the reader rather than routing them away from
 * the account they are reading.
 */
export function TaskDetailModal({
  activityId,
  readOnly,
  onClose,
  update,
}: Readonly<{
  activityId: string;
  // An archived company takes no new activity, so the verbs below would only
  // be refused server-side — omitted here rather than disabled and left
  // visible.
  readOnly: boolean;
  onClose: () => void;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const titleId = useId();
  const query = useQuery({
    queryKey: ["activity", activityId],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities/{id}", {
        params: { path: { id: activityId } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
  });
  const task: Activity | undefined = query.data;
  return (
    <Modal open onClose={onClose} labelledBy={titleId}>
      <h2 id={titleId} className="t-h2 modal-title">
        {task?.subject ?? t("tasks.detail")}
      </h2>
      {query.isPending && <PendingBody label={t("tasks.detailLoading")} />}
      {query.isError && (
        <p className="t-caption form-error">
          {problemMessageOf(query.error, t)}
        </p>
      )}
      {task && (
        <div className="form-stack">
          {task.body && <p className="t-body">{task.body}</p>}
          <p className="t-caption task-detail-meta">
            {task.due_at ? (
              <span>
                {t("co.next.due", {
                  // The one viewer-clock reading on this record surface, and it
                  // is not a preference: `dueInstant` mints a due date as the
                  // end of the picked day in the BROWSER's zone, so the stored
                  // instant already carries the picker's clock. Read in the
                  // organization's zone it names a different calendar day than
                  // the one the picker chose, for every reader outside that
                  // zone — there is no organization reading of it to prefer.
                  when: formatDate(task.due_at, locale, viewerZone()),
                })}
              </span>
            ) : (
              <span>{t("co.next.undated")}</span>
            )}
            <span>
              {t("tasks.logged")}{" "}
              {formatDateTime(task.occurred_at, locale, recordZone)}
            </span>
            {task.is_done && <Badge tone="success">{t("tasks.isDone")}</Badge>}
            {task.assignee_id && (
              <EntityRef kind="user" id={task.assignee_id} />
            )}
          </p>
          {!task.is_done && !readOnly && (
            <div className="form-actions">
              <TaskQuickActions
                activityId={task.id}
                version={task.version}
                dueAt={task.due_at}
                update={update}
                showDuePicker
              />
            </div>
          )}
        </div>
      )}
    </Modal>
  );
}

// useNoticeRead settles one notice — the acknowledge verb's whole meaning.
// Its own hook beside useTaskUpdate because the two route to different
// owners: a task's verbs go to the activity, a notice's one verb goes to
// the notice's own read endpoint.
export function useNoticeRead(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.POST("/notices/{id}/read", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

/**
 * Running one failed automation firing again.
 *
 * It resolves with the server's answer rather than throwing on a refusal,
 * because a refusal is a fact about the run and not a fault: the firing was
 * blocked on purpose, or its handler has not been established safe to repeat,
 * or its trigger event cannot be rebuilt. Each of those is something to TELL
 * the reader, and an error path would have to reconstruct which one it was.
 */
export function useAutomationRetry(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { data, error } = await api.POST("/automations/runs/{id}/retry", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

/**
 * Recording how a meeting went.
 *
 * Its own hook rather than a wider `useTaskUpdate`: that one's body is
 * task-shaped (is_done, due_at, remind_at), and a meeting outcome is a
 * different fact about a different kind of activity. Widening it would let a
 * caller send `is_done` for a meeting, which the server refuses with a
 * `field_not_valid_for_kind` fault — a refusal the types can prevent instead.
 */
export function useMeetingOutcome(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      // The version the answer was decided against, for the reason the task
      // verb beside it carries one: two readers answering the same meeting
      // would otherwise both succeed and the later one would win silently.
      version: number | undefined;
      status: "held" | "no_show" | "canceled";
    }) => {
      const { error } = await api.PATCH("/activities/{id}", {
        params: {
          path: { id: input.id },
          ...ifMatch(requireVersion(input.version)),
        },
        body: { meeting_status: input.status },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    onSuccess: (_data, input) => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
      // The meeting's own detail read too: a drawer open on it would otherwise
      // keep showing the row as unanswered after it has been answered.
      queryClient.invalidateQueries({ queryKey: ["activity", input.id] });
    },
  });
}

/**
 * Settling a promise: it was kept, or it no longer stands.
 *
 * Its own hook rather than useTaskUpdate's: a claim is not an activity, and the
 * two settle through different endpoints. `done` and `dismissed` are kept apart
 * by the server because they answer different questions later — how many
 * commitments this workspace keeps is a fact about the team, and how many
 * extracted claims were never real is a fact about the extractor.
 */
export function useClaimSettle(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      outcome: "done" | "dismissed";
    }) => {
      const { error } = await api.POST("/claims/{id}/settle", {
        params: { path: { id: input.id } },
        body: { outcome: input.outcome },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
      // The person's own card lists the same open claims, so a drawer standing
      // on them would keep showing a promise that has just been settled.
      queryClient.invalidateQueries({ queryKey: ["person"] });
    },
  });
}
