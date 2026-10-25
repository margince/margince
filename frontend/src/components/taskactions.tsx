import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useId, useRef } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  Modal,
  PendingBody,
} from "../design-system/atoms";
import { DateInput } from "../design-system/dateinput";
import { focusableWithin } from "../design-system/dialogfocus";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { Markdown } from "../design-system/markdown";
import { provenanceOf } from "../design-system/provenanceof";
import { SourceEvidence } from "../design-system/sourceevidence";
import { formatActivityWhen } from "../format/activitywhen";
import { calendarDay, dueInstant } from "../format/calendarday";
import { isoOrBlank } from "../format/datepattern";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemCodeOf, unwrap } from "../lib/problem";
import { useRecordZone } from "../lib/recordzone";
import { EntityRef } from "./entityref";
import { mostSpecificFirst, RecordLinks } from "./recordlinks";
import "./taskactions.css";
import { ErrorLine } from "../design-system/errorline";
import { useActivity } from "../lib/activityread";
import { CallStartProvider } from "../lib/callstart";
import { useTaskContact } from "../lib/taskcontact";
import { CallLogInline, useCallLogFor } from "./calllog.host";
import { useCompleteTask } from "./taskcomplete";
import { TaskNextSteps } from "./tasknextsteps";
import { refetchAfterWrite } from "./taskwritefollowup";

// Acting on a task from the record it belongs to. The tasks screen owns the
// standing work queue; this is the same two verbs (complete, snooze) offered
// where the rep already is, plus the detail a next-step row has no room for.
//
// Which cached reads a task write invalidates depends on where it was written
// from, so the caller passes them in — the work queue is workspace-wide, the
// record's timeline is not.

export {
  useAutomationRetry,
  useClaimSettle,
  useMeetingOutcome,
  useNoticeRead,
} from "./taskactions.rowwrites";

type Activity = components["schemas"]["Activity"];

type TaskPatch = {
  id: string;
  // The version the press was decided against. Every verb on a task goes
  // through this one mutation, so pinning it here pins all four at once — and
  // an unpinned tick is a task two contacts can complete, each told it worked.
  version: number | undefined;
  body: { is_done?: boolean; due_at?: string; remind_at?: string | null };
};

export function useTaskUpdate(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: TaskPatch) => {
      const data = unwrap(
        await api.PATCH("/activities/{id}", {
          params: {
            path: { id: input.id },
            ...ifMatch(requireVersion(input.version)),
          },
          body: input.body,
        }),
        t,
      );
      // The version the write PRODUCED, answered so a follow-on press has one.
      // An undo re-sending the version the row was drawn at would be refused as
      // skew by the very write it is undoing.
      return data?.version;
    },
    // Returned, so the mutation stays pending until the reads have landed. The
    // row the press came from stays drawn at the version it was fetched at
    // until then, and a second press in that window re-sends it and is
    // refused as skew by the write that has just succeeded.
    onSuccess: (_data, input) =>
      refetchAfterWrite(queryClient, invalidateKeys, input.id),
    // A stale press is not a failed one: the task moved on under it, so the
    // reads are refreshed to show where it is now.
    onError: (error, input) => {
      if (problemCodeOf(error) === "version_skew") {
        return refetchAfterWrite(queryClient, invalidateKeys, input.id);
      }
    },
  });
}

/**
 * The next due date one snooze away, or null for a task that has no date to
 * move.
 *
 * A snooze moves the task to the NEXT CALENDAR DAY, which is not the same as
 * adding twenty-four hours. A local day is not always that long: on Europe's
 * spring-forward day, adding a day to the 28th at 23:59:59 lands at 00:59:59
 * on the 30th, so a rep pressing "tomorrow" skips the 29th entirely. The
 * accept path warns about exactly this arithmetic where it stamps a deadline;
 * this writer was doing it.
 *
 * Read and re-minted through the calendar helpers, in the zone the deadline
 * belongs to, so a snooze lands on the day it names for every colleague.
 */
export function snoozedDueAt(
  dueAt: string | null | undefined,
  zone: string,
): string | null {
  if (!dueAt) {
    return null;
  }
  const today = calendarDay(new Date(dueAt), zone);
  const [year = Number.NaN, month = Number.NaN, day = Number.NaN] = today
    .split("-")
    .map(Number);
  // Through UTC parts, which is a pure calendar step: no zone reading happens
  // here, so no DST transition can shorten or lengthen the day being counted.
  const next = new Date(Date.UTC(year, month - 1, day + 1))
    .toISOString()
    .slice(0, "yyyy-mm-dd".length);
  return dueInstant(next, zone);
}

/**
 * TaskCompleteCheck is the tick itself: a checkbox for a row that names its
 * own verb, since ticking is completing. Every caller lists open tasks only,
 * so the box starts unchecked. The row leaves on the invalidated re-read,
 * with the confirmation and its Undo raised meanwhile.
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
  const complete = useCompleteTask(update);
  const isThisTask = update.variables?.id === activityId;
  const pending = update.isPending && isThisTask;
  // A rejected PATCH re-enables the box, unchecked, like a click that did
  // nothing; this line is what tells the two apart.
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
        onChange={() => complete(activityId, version)}
      />
      {failed && <ErrorLine error={update.error} inline />}
    </>
  );
}

/** Complete, snooze a dated task, or explicitly choose its next due day. */
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
  // Off by default, since a date box in every next-steps row is too heavy.
  // The detail modal opts in: a reader came there to work on one task.
  showDuePicker?: boolean;
}>) {
  const t = useT();
  const recordZone = useRecordZone();
  const nextDue = snoozedDueAt(dueAt, recordZone);
  const pending = update.isPending && update.variables?.id === activityId;
  const complete = useCompleteTask(update);
  return (
    <>
      {showComplete && (
        <Button
          variant="primary"
          disabled={pending}
          onClick={() => complete(activityId, version)}
        >
          {t("tasks.complete")}
        </Button>
      )}
      {nextDue && (
        <Button
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
// Beside the snooze, not instead of it. Snooze adds a day to the task's own due
// date, so a task three days overdue only moves to two days overdue.
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
  const recordZone = useRecordZone();
  // Seeded from the task's own day in the INSTALLATION's zone, matching what
  // the meta line above reads it in: a picker opening on a different day than
  // the one displayed beside it would be two answers to "when is this due".
  const current = isoOrBlank(
    dueAt ? calendarDay(new Date(dueAt), recordZone) : "",
  );
  return (
    <Field label={t("tasks.moveTo")}>
      {(field) => (
        <DateInput
          {...field}
          value={current}
          disabled={pending}
          onChange={(event) => {
            const day = event.target.value;
            // A cleared box is not a request to undate the task.
            if (day === "") {
              return;
            }
            update.mutate({
              id: activityId,
              version,
              body: { due_at: dueInstant(day, recordZone) },
            });
          }}
        />
      )}
    </Field>
  );
}

/**
 * TaskDetailDialog opens one task where it was listed.
 *
 * A task has no screen of its own — it lives in a timeline, not on a record
 * page — so the detail comes to the reader rather than routing them away from
 * the account they are reading.
 */
export function TaskDetailDialog({
  activityId,
  readOnly,
  onClose,
  update,
  onOpenSource,
}: Readonly<{
  activityId: string;
  // An archived company takes no new activity, so the verbs below would only
  // be refused server-side — omitted here rather than disabled and left
  // visible.
  readOnly: boolean;
  onClose: () => void;
  update: ReturnType<typeof useTaskUpdate>;
  onOpenSource: (id: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const titleId = useId();
  const query = useActivity(activityId);
  const task: Activity | undefined = query.data;
  // Logged here rather than in the shell's drawer, which would cover the task.
  const callLog = useCallLogFor(useTaskContact(task).contact?.id);
  const body = useRef<HTMLDivElement>(null);
  const verbs = useRef<HTMLDivElement>(null);
  // The task's own verbs, else the box: Enter on a first-focused link or Call
  // would leave the dialog or start a call.
  const firstFocus = () =>
    (verbs.current && focusableWithin(verbs.current)[0]) ??
    body.current?.closest<HTMLElement>('[role="dialog"]') ??
    null;
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer"
      initialFocusTo={firstFocus}
    >
      <DrawerHead className="task-detail-head">
        <Heading size="large" id={titleId} className="t-h2">
          {task?.subject ?? t("tasks.detail")}
        </Heading>
      </DrawerHead>
      <DrawerBody ref={body}>
        <CallStartProvider start={callLog.start}>
          {query.isPending && <PendingBody label={t("tasks.detailLoading")} />}
          <ErrorLine error={query.error} />
          {task && (
            <div className="form-stack">
              {/* Markdown, as the timeline draws the same body. */}
              {task.body && (
                <div className="t-body task-detail-body">
                  <Markdown source={task.body} autolink />
                </div>
              )}
              <div className="t-caption task-detail-meta">
                {/* In the same tab: the reader opened the panel to learn what the
                  task is about, and following its record is leaving for it. */}
                {task.links && task.links.length > 0 && (
                  <span>
                    <RecordLinks links={mostSpecificFirst(task.links)} />
                  </span>
                )}
                {task.due_at ? (
                  <span>
                    {t("co.next.due", {
                      // The record's own clock, like every other date on this
                      // surface. A deadline is a fact colleagues read back, so its
                      // day cannot depend on where the reader sits. `dueInstant`
                      // mints the picked day's end in this zone; the browser's
                      // zone would show an approved 9 September as due the 10th.
                      when: formatDate(task.due_at, locale, recordZone),
                    })}
                  </span>
                ) : (
                  <span>{t("co.next.undated")}</span>
                )}
                <span>
                  {t("tasks.logged")}{" "}
                  {formatActivityWhen(
                    task.occurred_at,
                    task.occurred_precision,
                    locale,
                    recordZone,
                  )}
                </span>
                {task.is_done && (
                  <Badge tone="success">{t("tasks.isDone")}</Badge>
                )}
                {task.assignee_id && (
                  <EntityRef kind="user" id={task.assignee_id} />
                )}
                {/* A task an agent wrote says so, so a reader knows it was
                  read out of a conversation rather than typed by a colleague. */}
                {provenanceOf(task.captured_by).kind === "agent" && (
                  <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
                )}
              </div>
              <TaskNextSteps
                task={task}
                className="form-actions task-detail-actions"
              />
              {!task.is_done && !readOnly && (
                <div ref={verbs} className="form-actions task-detail-actions">
                  <TaskQuickActions
                    activityId={task.id}
                    version={task.version}
                    dueAt={task.due_at}
                    update={update}
                    showDuePicker
                  />
                </div>
              )}
              {callLog.call && (
                <CallLogInline
                  key={callLog.call.entityId}
                  call={callLog.call}
                  onClose={callLog.close}
                />
              )}
              {/* The evidence last, under the verbs. A task's own verbs are what
                the reader came to press, and a message long enough to scroll
                would otherwise push them off the panel. */}
              {task.source_activity_id && (
                <SourceEvidence
                  activityId={task.source_activity_id}
                  onOpenTranscript={onOpenSource}
                />
              )}
            </div>
          )}
        </CallStartProvider>
      </DrawerBody>
    </Modal>
  );
}
