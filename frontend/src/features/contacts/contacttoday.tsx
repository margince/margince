import { CheckSquare, FileText, Search, Send, Users } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { interactionIcon } from "../../components/interactionchrome";
import { useActivityGate } from "../../components/activitygate";
import { RecordNextBar } from "../../components/latestinbound";
import { taskLine, useTaskHolder } from "../../components/taskassignee";
import { TaskCallAction } from "../../components/tasknextsteps";
import { Button, type ButtonVariant } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { SourceLink } from "../../design-system/sourcelink";
import { formatDue } from "../../format/format";
import { daysPast } from "../../format/lateness";
import { useLocale, usePlural, useT } from "../../i18n";
import { latePromise } from "../../lib/latepromise";
import { useSourceOpener } from "../../lib/opensource";
import { tasksPeek } from "../../lib/standingpeeks";
import { taskDetail } from "../../lib/taskdetail";
import { MoveButton } from "../records/movebutton";
import { basisAddsARecord } from "../records/record360";
import { MayBeDoneVerbs, useMomentSetAside } from "./contactmaybedone";
import { momentTask, owedLateness, recordsTheMoveNames } from "./contactowed";
import { momentDrafts } from "./contactpage.drawers";
import "../records/record360/record360.css";
import { useViewerZone } from "../../lib/viewerzone";
import { openContactTab } from "../worklist/worklist.return";

// The one thing next on a contact, as the bar at the top of its Overview. It is the
// move the server's ladder picked, else the record's most urgent open task.
// The rest of the open tasks sit behind "+N tasks", which opens the queue.

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];
type ContactMomentAction = components["schemas"]["ContactMomentAction"];
type Activity = components["schemas"]["Activity"];

function actionableMoment(moment: ContactMoment | undefined): boolean {
  return Boolean(
    moment &&
      moment.rule !== "thin_relationship" &&
      moment.rule !== "nothing_needed",
  );
}

export function ContactToday({
  moment,
  view,
  onAction,
  onOpenTasks,
  onOpenEmail,
  onAddTask,
}: Readonly<{
  moment?: ContactMoment;
  view: Contact360;
  onAction: (action: ContactMomentAction) => void;
  onOpenTasks?: () => void;
  onOpenEmail?: (activityId: string) => void;
  // Opens the task form on this contact; absent where the reader may not log.
  onAddTask?: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const move = moment && actionableMoment(moment) ? moment : undefined;
  const named = recordsTheMoveNames(move);
  const tasks = (view.next_steps?.data ?? []).filter(
    (task) => !task.is_done && !named.has(task.id),
  );
  // The tasks behind "+N tasks": a row leads to the queue, which holds them all.
  const more = (rows: readonly Activity[]) =>
    onOpenTasks
      ? {
          count: rows.length,
          onOpen: onOpenTasks,
          peek: tasksPeek(
            rows.map((row) => ({
              id: row.id,
              subject: row.subject ?? t("task.untitled"),
              dueAt: row.due_at,
              overdue: row.due_at
                ? daysPast(Date.parse(row.due_at), Date.parse(view.as_of)).late
                : false,
            })),
            { t, plural, locale, zone },
            rows.length,
            onOpenTasks,
          ),
        }
      : undefined;
  if (move) {
    return (
      <MomentBar
        moment={move}
        view={view}
        onAction={onAction}
        onOpenEmail={onOpenEmail}
        more={more(tasks)}
      />
    );
  }
  const [task, ...rest] = tasks;
  if (task) {
    return <TaskBar task={task} view={view} more={more(rest)} />;
  }
  return (
    <AnswerBar
      moment={moment}
      view={view}
      onAction={onAction}
      onAddTask={onAddTask}
    />
  );
}

type More = Readonly<{ count: number; onOpen: () => void }> | undefined;

const URGENT_RULES: ReadonlySet<ContactMoment["rule"]> = new Set([
  "privacy_notice_due",
  "meeting_prep",
  "overdue_promise",
]);

// The server's `why_now` is the only reason: one moment gives one reason, and
// the facts in it are the part a rep judges.
function MomentBar({
  moment,
  view,
  onAction,
  onOpenEmail,
  more,
}: Readonly<{
  moment: ContactMoment;
  view: Contact360;
  onAction: (action: ContactMomentAction) => void;
  onOpenEmail?: (activityId: string) => void;
  more: More;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const late = owedLateness(moment, view, zone, { t, plural, locale });
  const notNow = useMomentSetAside(view.contact.id);
  const defer = notNow.deferOf(moment);
  const task = momentTask(moment, view);
  const call = task && (
    <TaskCallAction task={task} pageContact={view.contact} />
  );
  return (
    <RecordNextBar
      // A meeting about to happen, or a promise past its date, outranks what
      // just came in; the item then sits above the bar.
      urgent={URGENT_RULES.has(moment.rule) || undefined}
      about={moment.evidence.map((item) => item.id)}
      title={moment.headline}
      late={late ? { label: late.badge, tone: "danger" } : undefined}
      // A late promise's sentence counts the same calendar days as its badge;
      // the server's counts elapsed days.
      sub={
        moment.rule === "overdue_promise" && late
          ? late.sentence
          : moment.why_now
      }
      insight={
        basisAddsARecord(moment) ? (
          <MomentSource moment={moment} view={view} onOpenEmail={onOpenEmail} />
        ) : undefined
      }
      action={
        moment.may_be_done ? (
          <>
            <MayBeDoneVerbs
              moment={moment}
              question={moment.may_be_done}
              view={view}
            />
            {call}
          </>
        ) : (
          <>
            <ActionVerb
              action={moment.recommended_action}
              contact={view.contact}
              primary
              onAction={onAction}
            />
            {call}
            {notNow.failure && <ErrorLine error={notNow.failure} inline />}
          </>
        )
      }
      // The may-be-done verbs carry their own "Not yet", which is this same
      // dismissal, so a snooze beside them would offer one act twice.
      snooze={
        moment.may_be_done || moment.rule === "privacy_notice_due"
          ? undefined
          : { onSnooze: defer.onDefer, pending: defer.pending }
      }
      more={more}
    />
  );
}

// The first record the move rests on that its headline does not name, as a
// link to it.
function MomentSource({
  moment,
  view,
  onOpenEmail,
}: Readonly<{
  moment: ContactMoment;
  view: Contact360;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const openerFor = useSourceOpener(onOpenEmail);
  const item = moment.evidence.find((entry) => entry.type === "activity");
  if (!item) {
    return null;
  }
  // A source the page has read opens in its reader. Any other source, or one
  // no reader may open, leads to the History tab.
  const activity = view.activities?.data.find((row) => row.id === item.id);
  const inPlace =
    activity &&
    openerFor({
      id: activity.id,
      kind: activity.kind,
      contentState: activity.content_state,
    });
  const open = inPlace ?? (() => openContactTab(view.contact.id, "timeline"));
  return (
    <>
      <span>{t("co.suggest.basedOn")}</span>{" "}
      <SourceLink icon={interactionIcon(activity?.kind)} onOpen={open}>
        {item.label}
      </SourceLink>
    </>
  );
}

function TaskBar({
  task,
  view,
  more,
}: Readonly<{ task: Activity; view: Contact360; more: More }>) {
  const t = useT();
  const plural = usePlural();
  const holder = useTaskHolder(task.assignee_id);
  const { locale } = useLocale();
  const zone = useViewerZone();
  const late =
    task.due_at &&
    latePromise(task.due_at, view.as_of, zone, "task", { t, plural, locale });
  return (
    <RecordNextBar
      title={task.subject ?? t("task.untitled")}
      late={late ? { label: late.badge, tone: "danger" } : undefined}
      detail={taskDetail(task)}
      sub={taskLine([
        holder,
        task.due_at
          ? t("co.next.due", {
              when: formatDue(task.due_at, task.due_has_time, locale, zone),
            })
          : t("co.next.undated"),
      ])}
      action={
        <>
          <TaskCallAction task={task} pageContact={view.contact} />
          <MoveButton
            contactId={view.contact.id}
            move={{ action: "open_task", arguments: { activity_id: task.id } }}
          />
        </>
      }
      more={more}
    />
  );
}

// No move and no task. A quiet rung on a record whose tasks the reader saw in
// full says no follow-up task is recorded, and no more: it does not claim that
// nothing is left to do. An unread reading, a withheld source and a
// relationship with nothing recorded say what they do know instead.
function AnswerBar({
  moment,
  view,
  onAction,
  onAddTask,
}: Readonly<{
  moment: ContactMoment | undefined;
  view: Contact360;
  onAction: (action: ContactMomentAction) => void;
  onAddTask?: () => void;
}>) {
  const t = useT();
  const omitted = new Set(view.sections_omitted);
  const withheld = [
    ...(omitted.has("moments") ? [t("today.source.moments")] : []),
    ...(omitted.has("next_steps") ? [t("today.source.nextSteps")] : []),
  ];
  if (withheld.length > 0) {
    return (
      <RecordNextBar
        title={t("today.withheld", { sections: withheld.join(", ") })}
      />
    );
  }
  if (!moment) {
    return <RecordNextBar title={t("record.notShown")} />;
  }
  if (moment.rule === "thin_relationship") {
    return (
      <RecordNextBar
        title={moment.headline}
        sub={t("contact.overview.coverage")}
        action={
          <ActionVerb
            action={moment.recommended_action}
            contact={view.contact}
            onAction={onAction}
          />
        }
      />
    );
  }
  return (
    <RecordNextBar
      // "What was said" already shows the last exchange, of every kind.
      lastExchange={false}
      empty={t("standing.next.noTask")}
      action={
        onAddTask && (
          <Button variant="ghost" onClick={onAddTask}>
            {t("standing.next.addTask")}
          </Button>
        )
      }
    />
  );
}

// The bar has one filled verb, its lead; indigo when the model writes it.
function verbVariant(drafts: boolean, lead: boolean): ButtonVariant {
  if (lead) return drafts ? "ai" : "primary";
  return drafts ? "aiQuiet" : "ghost";
}

function ActionVerb({
  action,
  contact,
  primary,
  onAction,
}: Readonly<{
  action: ContactMomentAction;
  contact: Contact360["contact"];
  primary?: boolean;
  onAction: (action: ContactMomentAction) => void;
}>) {
  const t = useT();
  const gate = useActivityGate(
    contact,
    action.kind === "schedule_meeting"
      ? "scheduling.bookRefused"
      : "record.logActivityRefused",
  );
  const attaches =
    action.destination?.surface === "booking" ||
    action.destination?.surface === "activity_log";
  const blocked = action.state === "blocked";
  // A rule ladder picks the verb, not a model. Only the bar's lead verb is
  // filled; a press that has the model write the mail carries the ✦.
  const drafts = momentDrafts(action);
  const variant = verbVariant(drafts, primary === true);
  return (
    <span className="today-verb">
      <Button
        variant={variant}
        onClick={() => onAction(action)}
        disabled={attaches && gate.pending}
        reason={
          blocked
            ? blockedReason(action, t)
            : attaches && gate.refusalKey
              ? t(gate.refusalKey)
              : undefined
        }
      >
        {!drafts && actionIcon(action.kind)}
        {action.label}
      </Button>
    </span>
  );
}

// A moment says what to do; it never says what carries it. So the reply verb is
// drawn as a send and not as an envelope — the envelope told a contact reached
// only over a chat channel that the button would mail them, which is neither
// what the action does nor something this card can know.
function actionIcon(kind: string): ReactNode {
  switch (kind) {
    case "schedule_meeting":
    case "open_meeting_brief":
    case "ask_colleague":
      return <Users size={15} aria-hidden="true" />;
    case "draft_reply":
      return <Send size={15} aria-hidden="true" />;
    case "complete_task":
      return <CheckSquare size={15} aria-hidden="true" />;
    case "open_research":
      return <Search size={15} aria-hidden="true" />;
    default:
      return <FileText size={15} aria-hidden="true" />;
  }
}

function blockedReason(
  action: ContactMomentAction,
  t: ReturnType<typeof useT>,
): string {
  return action.blocked_reason ?? t("contact.rail.blocked");
}
