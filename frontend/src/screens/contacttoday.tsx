import {
  CalendarDays,
  CheckSquare,
  FileText,
  Search,
  Send,
  Users,
} from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { formatDate } from "../format/format";
import { daysPast } from "../format/lateness";
import { type Locale, useLocale, useT } from "../i18n";
import { contactTabRoute } from "./contacttab";
import { useRoster } from "./entityref";
import { interactionIcon } from "./interactionchrome";
import { MoveButton } from "./movebutton";
import { FoundMove, TodayPanel, TodoRow, WithheldNotice } from "./record360";

// The server selects the recommendation. Empty relationship and quiet results
// describe coverage; they do not describe outstanding work.

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];
type ContactMomentAction = components["schemas"]["ContactMomentAction"];
type Activity = components["schemas"]["Activity"];

export function hasContactWork(view: Contact360): boolean {
  return (
    actionableMoment(view.moment) ||
    (view.next_steps?.data.some((task) => !task.is_done) ?? false)
  );
}

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
  const taskRows = useOpenTaskRows(view);
  const omitted = new Set(view.sections_omitted ?? []);
  const withheld = (
    <WithheldNotice
      sections={[
        ...(omitted.has("moments") ? [t("today.source.moments")] : []),
        ...(omitted.has("next_steps") ? [t("today.source.nextSteps")] : []),
      ]}
    />
  );
  if (!actionableMoment(moment) && taskRows.length === 0) {
    return (
      <div className="contact-coverage t-sub">
        <p>
          {moment
            ? moment.rule === "thin_relationship"
              ? t("contact.moment.rule.thin_relationship")
              : moment.why_now
            : t("record.notShown")}
        </p>
        {moment?.rule === "thin_relationship" && (
          <p>{t("contact.overview.coverage")}</p>
        )}
        {withheld}
      </div>
    );
  }
  return (
    <TodayPanel
      onOpenTasks={onOpenTasks}
      tasksLabel={t("brief.feed.fullWorklist")}
      notice={withheld}
    >
      {moment && actionableMoment(moment) && (
        <MomentMove
          moment={moment}
          view={view}
          onAction={onAction}
          onOpenEmail={onOpenEmail}
        />
      )}
      {taskRows}
    </TodayPanel>
  );
}

// Evidence stays beside the action it supports.
function MomentMove({
  moment,
  view,
  onAction,
  onOpenEmail,
}: Readonly<{
  moment: ContactMoment;
  view: Contact360;
  onAction: (action: ContactMomentAction) => void;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const secondary = moment.secondary_actions ?? [];
  return (
    <FoundMove
      suggested
      title={moment.headline}
      basis={
        <ul className="pe-today-evidence">
          {[
            ...new Map(
              moment.evidence.map((item) => [
                `${item.type}:${item.id ?? item.label}`,
                item,
              ]),
            ).values(),
          ].map((item) => (
            <li
              key={`${item.type}-${item.id ?? item.label}`}
              className="t-body"
            >
              {evidenceIcon(item.type)}
              {item.id ? (
                <Button
                  small
                  variant="ghost"
                  onClick={() => {
                    const activity = view.activities?.data.find(
                      (row) => row.id === item.id,
                    );
                    if (activity?.kind === "email" && onOpenEmail && item.id)
                      onOpenEmail(item.id);
                    else navigate(contactTabRoute(view.contact.id, "timeline"));
                  }}
                >
                  {item.label}
                </Button>
              ) : (
                <span>{item.label}</span>
              )}
              {item.snippet && <q>{item.snippet}</q>}
            </li>
          ))}
        </ul>
      }
      action={
        <div className="pe-today-actions">
          <ActionVerb
            action={moment.recommended_action}
            primary
            onAction={onAction}
          />
          {/* Every other verb the moment carries, beside the one it leads with
              rather than in a second list elsewhere on the page, which would
              let the two disagree about what to do next. */}
          {secondary.map((action) => (
            <ActionVerb
              key={action.label}
              action={action}
              onAction={onAction}
            />
          ))}
        </div>
      }
    />
  );
}

// The open tasks already on this contact's record, quieter than the move
// above them: commitments the record already carries, which a reader scans.
//
// Rows rather than a component, because the panel counts what it is handed to
// decide whether the day is quiet — and a component that renders nothing is
// still one child. A withheld section yields no rows; the rail says what was
// withheld.
function useOpenTaskRows(view: Contact360): ReactNode[] {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const tasks = (view.next_steps?.data ?? []).filter((task) => !task.is_done);
  // The assignee's name off the workspace roster, so the row's mark is the
  // colleague it sits with rather than an id. Asked for only when a task names
  // one.
  const roster = useRoster(
    "user",
    tasks.some((task) => Boolean(task.assignee_id)),
  );
  const nameOf = (userId: string | null | undefined): string | undefined => {
    if (!userId) {
      return undefined;
    }
    const entry = roster.data?.find((candidate) => candidate.id === userId);
    return entry && "display_name" in entry ? entry.display_name : undefined;
  };
  const asOf = Date.parse(view.as_of);
  return tasks
    .slice(0, OPEN_TASKS_SHOWN)
    .map((task) => (
      <TodoRow
        key={task.id}
        who={nameOf(task.assignee_id)}
        title={task.subject ?? t("task.untitled")}
        due={taskDue(task, asOf, t, locale, zone)}
        action={
          <MoveButton
            contactId={view.contact.id}
            move={{ action: "open_task", arguments: { activity_id: task.id } }}
          />
        }
      />
    ));
}

// How many open tasks the day's work lists before the reader is sent to the
// full list. Three is what reads as a glance beside the move above them.
const OPEN_TASKS_SHOWN = 3;

// When a task is owed, coloured only once it is late. A task with no date says
// so rather than leaving the slot empty, which reads as a date that failed to
// load. Lateness is measured against the read's own `as_of`, so the row agrees
// with the dates beside it and does not drift while a tab is left open.
function taskDue(
  task: Activity,
  asOf: number,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): { label: string; tone?: "danger" } {
  if (!task.due_at) {
    return { label: t("co.next.undated") };
  }
  const { late } = daysPast(Date.parse(task.due_at), asOf);
  if (late) {
    return { label: t("co.next.overdue"), tone: "danger" };
  }
  return {
    label: t("co.next.due", { when: formatDate(task.due_at, locale, zone) }),
  };
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
  const state = readiness(action, t);
  return (
    <span className="pe-today-verb">
      {/* Readiness is stated rather than left to a disabled button, because
          "you may not do this yet" and "this will ask you to confirm" are
          different answers. A blocked verb hands its sentence to the Button,
          whose `reason` bars the press AND describes the control with it — a
          `title` on a disabled button reaches no screen reader. The other
          states keep the caption under the verb. */}
      <Button
        variant={primary ? "primary" : "ghost"}
        small
        onClick={() => onAction(action)}
        reason={blocked ? state : undefined}
      >
        {actionIcon(action.kind)}
        {action.label}
      </Button>
      {!blocked && state && (
        <span className="pe-today-verb-state t-caption">{state}</span>
      )}
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

// What pressing it will do, when that is not already obvious from the control.
// A blocked action says so in words, not only by being unpressable: a disabled
// button carries no title a keyboard or touch reader ever sees, so the
// server's own reason (WHY this one is blocked) renders here when it sent
// one, and the generic word is the fallback for the rare blocked action that
// carries none.
//
// An available verb says nothing. A pressable button IS the whole of "ready",
// and the word under it read as a status the record had reached — one more
// machine noun on a card that already carries three, under the one control a
// reader came to press.
function readiness(
  action: ContactMomentAction,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (action.state === "will_confirm") {
    return t("contact.rail.reviewFirst");
  }
  if (action.state === "blocked") {
    return action.blocked_reason ?? t("contact.rail.blocked");
  }
  return undefined;
}

// A moment's evidence names a record TYPE and nothing about the transport, so
// there is no honest envelope to draw beside an activity: it is as likely to
// be a chat message or a call as a mail, and on a contact reached only over a
// channel the envelope was simply wrong. It is drawn as the record it is.
function evidenceIcon(type: string): ReactNode {
  switch (type) {
    case "task":
      return <CalendarDays size={15} aria-hidden="true" />;
    case "relationship_change":
      return <Users size={15} aria-hidden="true" />;
    default:
      return interactionIcon(null, 15);
  }
}
