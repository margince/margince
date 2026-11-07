import { type ReactNode, useId, useRef } from "react";
import type { components } from "../api/schema";
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
import { NoteMarkdown } from "../design-system/markdown";
import { provenanceOf } from "../design-system/provenanceof";
import { SourceEvidence } from "../design-system/sourceevidence";
import { formatActivityWhen } from "../format/activitywhen";
import { calendarDay } from "../format/calendarday";
import { isoOrBlank } from "../format/datepattern";
import { formatDue } from "../format/format";
import { dueTimeOf, startOfDayInZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { taskDueFields } from "../lib/activitybody";
import { useCanWriteRecord } from "../lib/capability";
import { TaskTimeField } from "./followuptaskfields";
import { mostSpecificFirst, RecordLinks } from "./recordlinks";
import { TaskAssignee } from "./taskassignee";
import "./taskactions.css";
import { ErrorLine } from "../design-system/errorline";
import { useActivity } from "../lib/activityread";
import { CallStartProvider } from "../lib/callstart";
import type { SourceOpenerFor } from "../lib/opensource";
import { useTaskContact } from "../lib/taskcontact";
import { useViewerZone } from "../lib/viewerzone";
import { CallLogInline, useCallLogFor } from "./calllog.host";
import { useCompleteTask } from "./taskcomplete";
import { TaskNextSteps } from "./tasknextsteps";
import { taskOriginKey } from "./taskorigin";
import type { useTaskUpdate } from "./taskwrite";

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
export { useTaskDone, useTaskUpdate } from "./taskwrite";

type Activity = components["schemas"]["Activity"];

/** Keep the agreed wall time while moving at least as far as tomorrow. */
export function snoozedDueAt(
  dueAt: string | null | undefined,
  hasTime: boolean | undefined,
  zone: string,
  now = new Date(),
): { due_at: string; due_has_time: boolean } | null {
  if (!dueAt) return null;
  const dueDay = calendarDay(new Date(dueAt), zone);
  const today = calendarDay(now, zone);
  const base = dueDay > today ? dueDay : today;
  const next = calendarDay(new Date(startOfDayInZone(base, zone, 1)), zone);
  return taskDueFields(next, dueTimeOf(dueAt, hasTime, zone), zone);
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
  dueHasTime,
  update,
  showComplete = true,
  showDuePicker = false,
}: Readonly<{
  activityId: string;
  version: number | undefined;
  dueAt?: string | null;
  /** The task's `due_has_time`: whether `dueAt` is a time of day. */
  dueHasTime?: boolean;
  update: ReturnType<typeof useTaskUpdate>;
  showComplete?: boolean;
  // Whether the reader may name a day, rather than only step the due date on.
  // Off by default, since a date box in every next-steps row is too heavy.
  // The detail modal opts in: a reader came there to work on one task.
  showDuePicker?: boolean;
}>) {
  const t = useT();
  const readerZone = useViewerZone();
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
      {dueAt && (
        <Button
          disabled={pending}
          onClick={() => {
            const nextDue = snoozedDueAt(dueAt, dueHasTime, readerZone);
            if (nextDue)
              update.mutate({ id: activityId, version, body: nextDue });
          }}
        >
          {t("tasks.snooze")}
        </Button>
      )}
      {showDuePicker && (
        <TaskDueDatePick
          activityId={activityId}
          version={version}
          dueAt={dueAt}
          dueHasTime={dueHasTime}
          update={update}
        />
      )}
    </>
  );
}

// The picker also dates tasks that have no deadline to snooze.
function TaskDueDatePick({
  activityId,
  version,
  dueAt,
  dueHasTime,
  update,
}: Readonly<{
  activityId: string;
  version: number | undefined;
  dueAt?: string | null;
  dueHasTime?: boolean;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const pending = update.isPending && update.variables?.id === activityId;
  const readerZone = useViewerZone();
  // Seeded from the task's own day and time in the INSTALLATION's zone,
  // matching what the meta line above reads it in: a picker opening on a
  // different day than the one displayed beside it would be two answers to
  // "when is this due".
  const current = isoOrBlank(
    dueAt ? calendarDay(new Date(dueAt), readerZone) : "",
  );
  const time = dueAt ? dueTimeOf(dueAt, dueHasTime, readerZone) : "";
  const move = (day: string, nextTime: string) =>
    update.mutate({
      id: activityId,
      version,
      body: taskDueFields(day, nextTime, readerZone),
    });
  return (
    <>
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
              move(day, time);
            }}
          />
        )}
      </Field>
      {/* Clearing the time puts the task back to the end of its day. */}
      <TaskTimeField
        day={pending ? "" : current}
        value={time}
        onChange={(nextTime) => {
          if (current !== "") move(current, nextTime);
        }}
      />
    </>
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
  openerFor,
  why,
}: Readonly<{
  activityId: string;
  // An archived company takes no new activity, so the verbs below would only
  // be refused server-side — omitted here rather than disabled and left
  // visible.
  readOnly: boolean;
  onClose: () => void;
  update: ReturnType<typeof useTaskUpdate>;
  /** Opens the task's source whole; see `SourceEvidence`. */
  openerFor: SourceOpenerFor;
  // Why the task is on the list it was opened from, which only that list knows.
  why?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const titleId = useId();
  const query = useActivity(activityId);
  const task: Activity | undefined = query.data;
  const origin = task ? taskOriginKey(task) : null;
  // The fetched activity carries the server's per-row answer, so the dialog
  // decides for every host whether its changes would be admitted.
  const rowWritable = useCanWriteRecord("activity", task);
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
              {origin && <p className="t-body">{t(origin)}</p>}
              {/* Markdown, as the timeline draws the same body. */}
              {task.body && (
                <div className="t-body task-detail-body">
                  <NoteMarkdown source={task.body} />
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
                      when: formatDue(
                        task.due_at,
                        task.due_has_time,
                        locale,
                        readerZone,
                      ),
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
                    readerZone,
                  )}
                </span>
                {task.is_done && (
                  <Badge tone="success">{t("tasks.isDone")}</Badge>
                )}
                <TaskAssignee id={task.assignee_id} />
                {/* A task an agent wrote says so, so a reader knows it was
                  read out of a conversation rather than typed by a colleague. */}
                {provenanceOf(task.captured_by).kind === "agent" && (
                  <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
                )}
              </div>
              {why}
              <TaskNextSteps
                task={task}
                className="form-actions task-detail-actions"
              />
              {!task.is_done && !readOnly && rowWritable && (
                <div ref={verbs} className="form-actions task-detail-actions">
                  <TaskQuickActions
                    activityId={task.id}
                    version={task.version}
                    dueAt={task.due_at}
                    dueHasTime={task.due_has_time}
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
                  openerFor={openerFor}
                />
              )}
            </div>
          )}
        </CallStartProvider>
      </DrawerBody>
    </Modal>
  );
}
