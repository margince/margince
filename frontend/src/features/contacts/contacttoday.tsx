import { CheckSquare, FileText, Search, Send, Users } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { RecordNextBar } from "../../components/latestinbound";
import { NextTaskSlot } from "../../components/nexttaskslot";
import { TaskCallAction } from "../../components/tasknextsteps";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { formatDate, formatNumber } from "../../format/format";
import { daysPast } from "../../format/lateness";
import {
  type Locale,
  translatePlural,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { useSourceOpener } from "../../lib/opensource";
import { useRecordZone } from "../../lib/recordzone";
import { tasksPeek } from "../../lib/standingpeeks";
import { taskDetail } from "../../lib/taskdetail";
import { MoveButton } from "../records/movebutton";
import { basisAddsARecord } from "../records/record360";
import { MayBeDoneVerbs, useMomentSetAside } from "./contactmaybedone";
import "../records/record360/record360.css";
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
}: Readonly<{
  moment?: ContactMoment;
  view: Contact360;
  onAction: (action: ContactMomentAction) => void;
  onOpenTasks?: () => void;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
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
  return <AnswerBar moment={moment} view={view} onAction={onAction} />;
}

type More = Readonly<{ count: number; onOpen: () => void }> | undefined;

const URGENT_RULES: ReadonlySet<ContactMoment["rule"]> = new Set([
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
  const { locale } = useLocale();
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
      late={lateness(owedDue(moment, view), view, t, locale)}
      sub={moment.why_now}
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
        moment.may_be_done
          ? undefined
          : { onSnooze: defer.onDefer, pending: defer.pending }
      }
      more={more}
      onward={<NextTaskSlot />}
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
      <Button variant="link" className="btn-valuelabel" onClick={open}>
        {item.label}
      </Button>
    </>
  );
}

function TaskBar({
  task,
  view,
  more,
}: Readonly<{ task: Activity; view: Contact360; more: More }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (
    <RecordNextBar
      title={task.subject ?? t("task.untitled")}
      late={lateness(task.due_at, view, t, locale)}
      detail={taskDetail(task)}
      sub={
        task.due_at
          ? t("co.next.due", { when: formatDate(task.due_at, locale, zone) })
          : t("co.next.undated")
      }
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
      onward={<NextTaskSlot />}
    />
  );
}

// No move and no task. A quiet rung has the basis to say nothing is due. An
// unread reading, a withheld source and a relationship with nothing recorded
// do not, so each says what it does know instead.
function AnswerBar({
  moment,
  view,
  onAction,
}: Readonly<{
  moment: ContactMoment | undefined;
  view: Contact360;
  onAction: (action: ContactMomentAction) => void;
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
        onward={<NextTaskSlot />}
      />
    );
  }
  if (!moment) {
    return (
      <RecordNextBar title={t("record.notShown")} onward={<NextTaskSlot />} />
    );
  }
  if (moment.rule === "thin_relationship") {
    return (
      <RecordNextBar
        title={moment.headline}
        sub={t("contact.overview.coverage")}
        action={
          <ActionVerb action={moment.recommended_action} onAction={onAction} />
        }
        onward={<NextTaskSlot />}
      />
    );
  }
  return <RecordNextBar onward={<NextTaskSlot />} />;
}

// What the move says is owed, in the order both readers below take it: the
// promise it may already have settled, then its evidence.
function owedRecords(moment: ContactMoment): { id: string; task: boolean }[] {
  const promise = moment.may_be_done;
  return [
    ...(promise
      ? [{ id: promise.promise_id, task: promise.promise_type === "task" }]
      : []),
    ...moment.evidence.flatMap((item) =>
      item.id ? [{ id: item.id, task: item.type === "task" }] : [],
    ),
  ];
}

// The first listed task the move is about.
function momentTask(
  moment: ContactMoment,
  view: Contact360,
): Activity | undefined {
  const tasks = view.next_steps?.data ?? [];
  for (const record of owedRecords(moment)) {
    const task = record.task
      ? tasks.find((row) => row.id === record.id)
      : undefined;
    if (task) {
      return task;
    }
  }
  return undefined;
}

// The due date of what the move says is owed: a claim's or a listed task's.
function owedDue(moment: ContactMoment, view: Contact360): string | undefined {
  for (const { id } of owedRecords(moment)) {
    const due =
      view.claims?.find((claim) => claim.id === id)?.due_at ??
      view.next_steps?.data.find((task) => task.id === id)?.due_at;
    if (due) {
      return due;
    }
  }
  return undefined;
}

// Lateness against the read's own `as_of`, so the tag agrees with the dates
// beside it and does not drift while a tab is left open.
function lateness(
  due: string | null | undefined,
  view: Contact360,
  t: ReturnType<typeof useT>,
  locale: Locale,
): { label: string; tone: "danger" } | undefined {
  if (!due) {
    return undefined;
  }
  const { late, days } = daysPast(Date.parse(due), Date.parse(view.as_of));
  if (!late) {
    return undefined;
  }
  return {
    label:
      days > 0
        ? translatePlural(locale, "contact.standing.late", days, {
            count: formatNumber(days, locale),
          })
        : t("co.next.overdue"),
    tone: "danger",
  };
}

// The records the move is itself about. A task the move names is the subject
// of the bar, and counted again under "+N tasks" it would be owed twice.
function recordsTheMoveNames(move: ContactMoment | undefined): Set<string> {
  return new Set(
    (move?.evidence ?? []).flatMap((item) => (item.id ? [item.id] : [])),
  );
}

// One verb: the button, and under it what will happen when it is pressed.
function ActionVerb({
  action,
  primary,
  onAction,
}: Readonly<{
  action: ContactMomentAction;
  primary?: boolean;
  onAction: (action: ContactMomentAction) => void;
}>) {
  const t = useT();
  const blocked = action.state === "blocked";
  // The ladder is a rule, not a model, so the lead verb is the plain filled
  // button; indigo would claim an agent wrote it.
  const variant = primary ? "primary" : "ghost";
  return (
    <span className="today-verb">
      {/* A blocked verb hands its sentence to the Button, whose `reason`
          bars the press AND describes the control with it: a `title` on a
          disabled button reaches no screen reader. A verb that will ask for
          confirmation says nothing here; the confirmation IS the saying, and
          a caption announcing it under the button was the same step twice. */}
      <Button
        variant={variant}
        onClick={() => onAction(action)}
        reason={blocked ? blockedReason(action, t) : undefined}
      >
        {actionIcon(action.kind)}
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

// Why this verb may not be pressed: the server's own reason when it sent one,
// and the generic word for the rare blocked action that carries none.
function blockedReason(
  action: ContactMomentAction,
  t: ReturnType<typeof useT>,
): string {
  return action.blocked_reason ?? t("contact.rail.blocked");
}
