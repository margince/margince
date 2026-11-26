import { ChevronDown } from "lucide-react";
import {
  type ReactNode,
  type RefCallback,
  useId,
  useRef,
  useState,
} from "react";
import type { components } from "../api/schema";
import {
  Button,
  Checkbox,
  Modal,
  OverflowMenu,
  PendingBody,
} from "../design-system/atoms";
import { focusableWithin } from "../design-system/dialogfocus";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { SourceEvidence } from "../design-system/sourceevidence";
import { formatActivityWhen } from "../format/activitywhen";
import { useLocale, useT } from "../i18n";
import { useCanWriteRecord } from "../lib/capability";
import { isWebsiteInquiryTask } from "../lib/websiteinquiries";
import { TaskAssignee } from "./taskassignee";
import { TaskDetailReading, TaskDetailTitle } from "./taskdetailtext";
import "./taskactions.css";
import { ErrorLine } from "../design-system/errorline";
import { useActivity } from "../lib/activityread";
import { CallStartProvider } from "../lib/callstart";
import type { SourceOpenerFor } from "../lib/opensource";
import { useTaskContact } from "../lib/taskcontact";
import { useViewerZone } from "../lib/viewerzone";
import { CallLogInline, useCallLogFor } from "./calllog.host";
import { useCompleteTask } from "./taskcomplete";
import { TaskDetailMeta } from "./taskdetailmeta";
import { TaskNextStepsPending, useTaskNextSteps } from "./tasknextsteps";
import { taskOriginKey } from "./taskorigin";
import { TaskOutcomeForm } from "./taskoutcome";
import { TaskQuickActions } from "./taskquickactions";
import type { useTaskUpdate } from "./taskwrite";
import { TaskWriteFailure } from "./taskwritefailure";

// Acting on a task from the record it belongs to. The tasks screen owns the
// standing work queue; this is the same two verbs (complete, snooze) offered
// where the rep already is, plus the detail a next-step row has no room for.
//
// Which cached reads a task write invalidates depends on where it was written
// from, so the caller passes them in — the work queue is workspace-wide, the
// record's timeline is not.

export {
  readMeetingStatus,
  useAutomationRetry,
  useClaimSettle,
  useMeetingOutcome,
  useNoticeRead,
} from "./taskactions.rowwrites";
export { TaskDueDatePick } from "./taskduedatepick";
export { snoozedDueAt, TaskQuickActions } from "./taskquickactions";
export { useTaskDone, useTaskUpdate } from "./taskwrite";

type Activity = components["schemas"]["Activity"];

/**
 * TaskCompleteCheck is the tick itself: a checkbox for a row that names its
 * own verb, since ticking is completing. Its name is for screen readers only,
 * and names the task, so a box read out of its row still says what it does.
 * Every caller lists open tasks only, so the box starts unchecked. The row
 * leaves on the invalidated re-read, with the confirmation and its Undo raised
 * meanwhile.
 */
export function TaskCompleteCheck({
  activityId,
  subject,
  version,
  update,
}: Readonly<{
  activityId: string;
  subject: string;
  version: number | undefined;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const complete = useCompleteTask(update);
  const isThisTask = update.variables?.id === activityId;
  const pending = update.isPending;
  // A rejected PATCH re-enables the box, unchecked, like a click that did
  // nothing; this line is what tells the two apart.
  const failed = update.isError && isThisTask;
  const name = t("tasks.completeNamed", { subject });
  return (
    <>
      <Checkbox
        label={<span className="sr-only">{name}</span>}
        checked={false}
        disabled={pending}
        onChange={() => complete(activityId, version)}
      />
      {failed && <ErrorLine error={update.error} inline />}
    </>
  );
}

/**
 * TaskDetailDialog opens one task where it was listed.
 *
 * A task has no screen of its own; it lives in a timeline. So the detail comes
 * to the reader, who stays on the account they are reading. The head says what
 * the task is and holds its verbs. The body opens on why it is on the list,
 * then what it came from.
 */
export function TaskDetailDialog({
  activityId,
  readOnly,
  onClose,
  update,
  openerFor,
  why,
  title,
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
  // The name the opening card gave the task, so the two read alike.
  title?: string;
}>) {
  const t = useT();
  const titleId = useId();
  const query = useActivity(activityId);
  const task: Activity | undefined = query.data;
  // The fetched activity carries the server's per-row answer, so the dialog
  // decides for every host whether its changes would be admitted.
  const rowWritable = useCanWriteRecord("activity", task);
  // Logged here rather than in the shell's drawer, which would cover the task.
  const callLog = useCallLogFor(useTaskContact(task).contact?.id);
  const [logging, setLogging] = useState(false);
  const body = useRef<HTMLDivElement>(null);
  const own = useRef<HTMLSpanElement>(null);
  const canAct = Boolean(task && !task.is_done && !readOnly && rowWritable);
  // Done, else the box: Enter on a first-focused step could send mail or
  // start a call, and on More it would open a menu.
  const firstFocus = () =>
    (own.current && focusableWithin(own.current)[0]) ??
    body.current?.closest<HTMLElement>('[role="dialog"]') ??
    null;
  // Opening the log form unmounts the verbs that held focus, and closing it
  // unmounts the form. Each move puts focus where the reader now is; the
  // microtask waits for the verbs the close brings back.
  const closeLog = () => setLogging(false);
  const formMounted: RefCallback<HTMLDivElement> = (el) => {
    if (el) {
      const controls = focusableWithin(el);
      (controls.find((c) => c.tagName !== "BUTTON") ?? controls[0])?.focus();
    } else queueMicrotask(() => firstFocus()?.focus());
  };
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer"
      initialFocusTo={firstFocus}
    >
      <CallStartProvider start={callLog.start}>
        <DrawerHead className="task-detail-head">
          <Heading size="medium" as="h2" id={titleId}>
            {title ?? <TaskDetailTitle task={task} />}
          </Heading>
          {task && (
            <>
              <TaskDetailMeta task={task} update={update} canAct={canAct} />
              {!logging && (
                <TaskDetailVerbs
                  task={task}
                  canAct={canAct}
                  update={update}
                  own={own}
                  onLog={() => setLogging(true)}
                />
              )}
              {!logging && (
                <TaskWriteFailure taskId={task.id} update={update} />
              )}
            </>
          )}
        </DrawerHead>
        <DrawerBody ref={body}>
          {query.isPending && <PendingBody label={t("tasks.detailLoading")} />}
          <ErrorLine error={query.error} />
          {task && (
            <div className="form-stack task-detail-sections">
              {logging && (
                <div ref={formMounted}>
                  <TaskOutcomeForm
                    task={task}
                    onLogged={closeLog}
                    onBack={closeLog}
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
              {why}
              <TaskDetailContext task={task} openerFor={openerFor} />
              <TaskDetailFootnote task={task} />
            </div>
          )}
        </DrawerBody>
      </CallStartProvider>
    </Modal>
  );
}

/**
 * At most three verbs: the type's own step, Done, and More. More holds the rest
 * in one order: log what happened, the other way to reach them, open them.
 */
function TaskDetailVerbs({
  task,
  canAct,
  update,
  own,
  onLog,
}: Readonly<{
  task: Activity;
  canAct: boolean;
  update: ReturnType<typeof useTaskUpdate>;
  own: React.RefObject<HTMLSpanElement | null>;
  onLog: () => void;
}>) {
  const t = useT();
  // The type's own step is the panel's one filled verb, so Done steps back.
  const next = useTaskNextSteps(task, false, true);
  if (!canAct && next.steps === null && !next.others) return null;
  return (
    <div className="task-detail-verbs">
      {/* Done's weight waits on the contact read, so its place is held. */}
      {canAct && next.pending && <TaskNextStepsPending />}
      {next.steps}
      {canAct && (
        <span ref={own} className="task-detail-own">
          <TaskQuickActions
            showComplete={!next.pending}
            completeLabel={
              isWebsiteInquiryTask(task) ? t("inquiry.markHandled") : undefined
            }
            activityId={task.id}
            version={task.version}
            update={update}
            quiet={next.fills}
          />
        </span>
      )}
      <OverflowMenu
        label={t("tasks.more")}
        text={t("tasks.more")}
        face="button"
        icon={<ChevronDown aria-hidden="true" />}
      >
        {canAct && <Button onClick={onLog}>{t("tasks.log.action")}</Button>}
        {next.others}
      </OverflowMenu>
    </div>
  );
}

/** What the task came from: how it was made, its note, and its source. */
function TaskDetailContext({
  task,
  openerFor,
}: Readonly<{ task: Activity; openerFor: SourceOpenerFor }>) {
  const t = useT();
  const headingId = useId();
  const origin = taskOriginKey(task);
  if (!origin && !task.body && !task.source_activity_id) return null;
  return (
    <section className="task-detail-context" aria-labelledby={headingId}>
      <Heading size="medium" as="h3" id={headingId}>
        {t("tasks.context")}
      </Heading>
      {origin && <p className="t-body">{t(origin)}</p>}
      {task.body && <TaskDetailReading task={task} />}
      {task.source_activity_id && (
        <SourceEvidence
          activityId={task.source_activity_id}
          openerFor={openerFor}
        />
      )}
    </section>
  );
}

/** When the task was logged and who holds it, as the pane's last line. */
function TaskDetailFootnote({ task }: Readonly<{ task: Activity }>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  return (
    <p className="t-caption task-detail-foot">
      <span>
        {t("tasks.logged")}{" "}
        {formatActivityWhen(
          task.occurred_at,
          task.occurred_precision,
          locale,
          readerZone,
        )}
      </span>
      {/* A held task names its holder after a label, as Logged names its
          time; an unheld one reads Unassigned alone. */}
      {task.assignee_id ? (
        <span>
          {t("tasks.heldBy")} <TaskAssignee id={task.assignee_id} />
        </span>
      ) : (
        <TaskAssignee id={task.assignee_id} />
      )}
    </p>
  );
}
